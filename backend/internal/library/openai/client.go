// Package openai は OpenAI Responses API を薄くラップし、リトライ・タイムアウト・ログを 1 か所に持つ。
//
// プロンプト・JSON Schema・結果の解釈はアプリ側(internal/analysis/infrastructure など)の責務で、
// このパッケージは「1 リクエストを投げて envelope を返す」ことだけをする。
//
//	client, err := openai.New(openai.Options{APIKey: key, Logger: log})
//	resp, err := client.Responses(ctx, openai.Request{
//		Model:           "gpt-5-mini",
//		Instructions:    "...",
//		Input:           []openai.Input{openai.UserImageJPEG(jpegData, "high")},
//		Schema:          &openai.JSONSchema{Name: "receipt", Schema: schema, Strict: true},
//		ReasoningEffort: "low",
//		MaxOutputTokens: 4096,
//	})
//	// resp.OutputText を自分の構造体に Unmarshal する
//
// エラー:
//   - HTTP 4xx / 5xx は *APIError(status / type / code / message)。本文そのものは持たない
//   - 429 / 5xx / ネットワーク / 試行タイムアウト / status=failed は IsTemporary が true で、Options.Retry の範囲でリトライする
//   - リトライで解決しなかった場合も最後のエラーをそのまま返すので、呼び出し側は IsTemporary で分類できる
//
// ログ: 1 呼び出しを openai_request span にし、model / http_status_code / response_id / openai_request_id /
// input_tokens / output_tokens / reasoning_tokens を span_finished に載せる。4xx / 5xx では error object の
// type / code / message(切り詰め済み)も載せる。生のレスポンス本文と画像は出さない。
package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/retry"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

const (
	// DefaultBaseURL は OpenAI API のベース URL。
	DefaultBaseURL = "https://api.openai.com/v1"
	// SpanRequest は 1 呼び出しの span 名。
	SpanRequest = "openai_request"

	// maxResponseBytes はレスポンス本文の読み取り上限。
	maxResponseBytes = 10 << 20
	// maxErrorMessageRunes は APIError.Message に残す長さ。ログ 1 行を肥大化させない
	maxErrorMessageRunes = 500
)

// DefaultRetry は Options.Retry が空のときの方針。1,2,4,8 秒待ちで最大 5 回、1 回 60 秒まで。
var DefaultRetry = retry.Policy{
	Backoff:        retry.Exponential(time.Second, 8*time.Second, 4),
	AttemptTimeout: 60 * time.Second,
}

// Options は New に渡す設定。APIKey 以外は省略できる。
type Options struct {
	APIKey string
	// BaseURL は API のベース URL。空なら DefaultBaseURL。テストで httptest に向ける
	BaseURL string
	// HTTPClient は nil なら http.DefaultClient 相当。タイムアウトは ctx で制御するので Timeout は不要
	HTTPClient *http.Client
	// Retry は空(Backoff も AttemptTimeout も 0)なら DefaultRetry。ShouldRetry は常に IsTemporary
	Retry retry.Policy
	// Logger は nil なら何も出力しない
	Logger logger.Interface
	// Clock は nil なら実時刻。テストで待ちを飛ばす
	Clock timewrapper.Interface
}

// Client は Responses API のクライアント。
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
	policy  retry.Policy
	retrier *retry.Retrier
	log     logger.Interface
}

// New は Client を生成する。APIKey が空ならエラー。
func New(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("openai: APIKey is required")
	}
	log := opts.Logger
	if log == nil {
		log = logger.NewNop()
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	policy := opts.Retry
	if len(policy.Backoff) == 0 && policy.AttemptTimeout == 0 {
		policy = DefaultRetry
	}
	policy.ShouldRetry = IsTemporary

	return &Client{
		http:    httpClient,
		baseURL: baseURL,
		apiKey:  opts.APIKey,
		policy:  policy,
		retrier: retry.New(log, opts.Clock),
		log:     log,
	}, nil
}

// Request は Responses API への 1 リクエスト。
type Request struct {
	Model        string
	Instructions string
	Input        []Input
	// Schema を指定すると text.format = json_schema で構造化出力を要求する
	Schema *JSONSchema
	// ReasoningEffort は空なら送らない(モデル既定)
	ReasoningEffort string
	// MaxOutputTokens は 0 なら送らない(モデル既定)
	MaxOutputTokens int
	// Store は OpenAI 側に会話を保存するか。既定 false で送る
	Store bool
}

// Input は 1 メッセージ。
type Input struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

// Content はメッセージ内の 1 要素。Type は input_text / input_image。
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// JSONSchema は構造化出力の指定。
type JSONSchema struct {
	Name   string
	Schema map[string]any
	Strict bool
}

// UserText はテキスト 1 つの user メッセージを作る。
func UserText(text string) Input {
	return Input{Role: "user", Content: []Content{{Type: "input_text", Text: text}}}
}

// UserImageJPEG は JPEG 画像 1 枚の user メッセージを作る。detail は low / high / auto。
func UserImageJPEG(jpeg []byte, detail string) Input {
	return Input{Role: "user", Content: []Content{{
		Type:     "input_image",
		ImageURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg),
		Detail:   detail,
	}}}
}

// Response は Responses API の envelope。本文の解釈(OutputText の Unmarshal)は呼び出し側で行う。
type Response struct {
	ID string
	// Status は completed / incomplete。failed はエラーとして返るのでここには来ない
	Status string
	// IncompleteReason は Status が incomplete のときの理由(max_output_tokens など)
	IncompleteReason string
	// OutputText は最初の output_text。なければ空
	OutputText string
	// Refusal はモデルが拒否したときの文言。なければ空
	Refusal string
	Usage   Usage
	// RequestID は OpenAI 側のリクエスト ID(x-request-id)。問い合わせ用
	RequestID string
	// Raw はレスポンス本文そのもの。保存用。ログには出さない
	Raw []byte
}

// Usage はトークン使用量。
type Usage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}

// Responses は Responses API を呼ぶ。一時的なエラーは Options.Retry に従ってリトライする。
func (c *Client) Responses(ctx context.Context, req Request) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(c.buildBody(req))
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	fields := []logger.Field{logger.String("model", req.Model)}
	if req.ReasoningEffort != "" {
		fields = append(fields, logger.String("reasoning_effort", req.ReasoningEffort))
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanRequest, fields...)

	var resp *Response
	err = c.retrier.Do(ctx, SpanRequest, c.policy, func(ctx context.Context) error {
		r, err := c.once(ctx, body)
		if err != nil {
			return err
		}
		resp = r
		return nil
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			span.AddFields(
				logger.HTTPStatusCode(apiErr.StatusCode),
				logger.String("openai_request_id", apiErr.RequestID),
				logger.String("provider_type", apiErr.Type),
				logger.String("provider_code", apiErr.Code),
				logger.String("provider_message", apiErr.Message),
			)
		}
		span.End(err)
		return nil, err
	}

	span.End(nil,
		logger.HTTPStatusCode(http.StatusOK),
		logger.String("response_id", resp.ID),
		logger.String("openai_request_id", resp.RequestID),
		logger.String("response_status", resp.Status),
		logger.Int("input_tokens", resp.Usage.InputTokens),
		logger.Int("output_tokens", resp.Usage.OutputTokens),
		logger.Int("reasoning_tokens", resp.Usage.ReasoningTokens),
	)
	return resp, nil
}

func (c *Client) buildBody(req Request) map[string]any {
	body := map[string]any{
		"model": req.Model,
		"input": req.Input,
		"store": req.Store,
	}
	if req.Instructions != "" {
		body["instructions"] = req.Instructions
	}
	if req.MaxOutputTokens > 0 {
		body["max_output_tokens"] = req.MaxOutputTokens
	}
	if req.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": req.ReasoningEffort}
	}
	if req.Schema != nil {
		body["text"] = map[string]any{"format": map[string]any{
			"type":   "json_schema",
			"name":   req.Schema.Name,
			"strict": req.Schema.Strict,
			"schema": req.Schema.Schema,
		}}
	}
	return body
}

// once は 1 回の HTTP 呼び出し。
func (c *Client) once(ctx context.Context, body []byte) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	defer func() { _ = httpResp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return nil, &TransportError{Err: fmt.Errorf("read body: %w", err)}
	}
	requestID := httpResp.Header.Get("x-request-id")

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, apiErrorFrom(httpResp.StatusCode, requestID, raw)
	}
	return parseResponse(raw, requestID)
}

// envelope は Responses API の本文のうち、このパッケージが見る部分。
type envelope struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func parseResponse(raw []byte, requestID string) (*Response, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &TransportError{Err: fmt.Errorf("decode body: %w", err)}
	}
	if env.Status == "failed" {
		e := &ResponseFailedError{ID: env.ID, RequestID: requestID}
		if env.Error != nil {
			e.Type, e.Code, e.Message = env.Error.Type, env.Error.Code, truncate(env.Error.Message)
		}
		return nil, e
	}

	resp := &Response{
		ID:        env.ID,
		Status:    env.Status,
		RequestID: requestID,
		Raw:       raw,
		Usage: Usage{
			InputTokens:     env.Usage.InputTokens,
			OutputTokens:    env.Usage.OutputTokens,
			ReasoningTokens: env.Usage.OutputTokensDetails.ReasoningTokens,
		},
	}
	if env.IncompleteDetails != nil {
		resp.IncompleteReason = env.IncompleteDetails.Reason
	}
	for _, out := range env.Output {
		for _, c := range out.Content {
			switch {
			case c.Type == "refusal" && resp.Refusal == "":
				resp.Refusal = c.Refusal
			case c.Type == "output_text" && resp.OutputText == "":
				resp.OutputText = c.Text
			}
		}
	}
	return resp, nil
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) > maxErrorMessageRunes {
		return string(r[:maxErrorMessageRunes])
	}
	return s
}
