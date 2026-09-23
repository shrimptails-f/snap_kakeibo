package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/openai"
)

// receiptOutput は OpenAI の構造化出力(ReceiptSchema)の JSON。読めなかった項目は null。
// OpenAI 固有の形式なので domain には置かず、ここで domain.ReceiptReading へ変換する。
type receiptOutput struct {
	StoreName    *string        `json:"store_name"`
	PurchaseDate *string        `json:"purchase_date"`
	TotalAmount  *int64         `json:"total_amount"`
	Details      []detailOutput `json:"details"`
}

type detailOutput struct {
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Quantity int64  `json:"quantity"`
	Category string `json:"category"`
}

func (o receiptOutput) toReading() domain.ReceiptReading {
	details := make([]domain.ReadDetail, 0, len(o.Details))
	for _, d := range o.Details {
		details = append(details, domain.ReadDetail{Name: d.Name, Amount: d.Amount, Quantity: d.Quantity, Category: d.Category})
	}
	return domain.ReceiptReading{StoreName: o.StoreName, PurchaseDate: o.PurchaseDate, ReadAmount: o.TotalAmount, Details: details}
}

// Instructions は OpenAI に渡すプロンプト。
const Instructions = `レシート画像から店名・購入日・合計金額・明細を読み取り、明細を固定カテゴリに分類する。読めない項目はnullにし、推測で埋めない。購入日は時刻を含めずYYYY-MM-DD形式にする。金額は税込の整数（円）。合計金額は「合計」「お買上げ計」など支払額の行を使う。明細は商品行のみとし、小計・税・割引・預り金・お釣りは含めない。レシートでない画像ならdetailsを空にする。`

const (
	// DefaultRequestTimeout は 1 ジョブあたりの OpenAI 呼び出し(リトライ込み)の上限。
	// 5 分の試行 2 回と 1 秒の待機を収め、Lambda の 11 分上限より短くする
	DefaultRequestTimeout = 10*time.Minute + 30*time.Second
	maxOutputTokens       = 4096
	imageDetail           = "high"
	schemaName            = "receipt"
	responseStatusDone    = "completed"
	responseStatusPartial = "incomplete"
)

// ResponsesClient は OpenAI Responses API を呼ぶ契約。*openai.Client が満たす。
type ResponsesClient interface {
	Responses(ctx context.Context, req openai.Request) (*openai.Response, error)
}

var _ ResponsesClient = (*openai.Client)(nil)

// OpenAIReceiptAnalyzer は OpenAI Responses API の構造化出力でレシートを読み取る。
type OpenAIReceiptAnalyzer struct {
	Client          ResponsesClient
	Model           string
	ReasoningEffort string
	// Timeout は 0 なら DefaultRequestTimeout
	Timeout time.Duration
}

var _ application.ReceiptAnalyzer = OpenAIReceiptAnalyzer{}

// Analyze は画像を送り、応答を読み取り内容に変換する。
// 4xx など再試行しても解決しない応答は Failure として返し、一時的な失敗は error として返す。
func (a OpenAIReceiptAnalyzer) Analyze(ctx context.Context, jpeg []byte) (application.AnalyzerResponse, error) {
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := a.Client.Responses(ctx, openai.Request{
		Model:           a.Model,
		Instructions:    Instructions,
		Input:           []openai.Input{openai.UserImageJPEG(jpeg, imageDetail)},
		Schema:          &openai.JSONSchema{Name: schemaName, Schema: ReceiptSchema(), Strict: true},
		ReasoningEffort: a.ReasoningEffort,
		MaxOutputTokens: maxOutputTokens,
	})
	if err != nil {
		var apiErr *openai.APIError
		if errors.As(err, &apiErr) && !apiErr.Temporary() {
			// 認証・不正なリクエストなどは何度送っても同じなので、このアップロードの失敗として記録する。
			// 本文は保存しないので、調査に必要な error object の type / code を error_message に残す
			return application.AnalyzerResponse{Failure: failurePtr(domain.AnalysisFailed(clientErrorMessage(apiErr)))}, nil
		}
		return application.AnalyzerResponse{}, err
	}
	result := application.AnalyzerResponse{
		ResponseID: resp.ID,
		Raw:        resp.Raw,
		Usage: application.TokenUsage{
			InputTokens:     resp.Usage.InputTokens,
			OutputTokens:    resp.Usage.OutputTokens,
			ReasoningTokens: resp.Usage.ReasoningTokens,
		},
	}
	result.Reading, result.Failure = readingFromResponse(resp)
	return result, nil
}

func clientErrorMessage(apiErr *openai.APIError) string {
	msg := fmt.Sprintf("OpenAI APIがHTTP %dを返しました", apiErr.StatusCode)
	if apiErr.Type != "" || apiErr.Code != "" {
		msg += fmt.Sprintf(" (%s/%s)", apiErr.Type, apiErr.Code)
	}
	return msg
}

// readingFromResponse は応答本文を読み取り内容に変換する。採用できない応答は Failure を返す。
func readingFromResponse(resp *openai.Response) (domain.ReceiptReading, *domain.FailureReason) {
	switch {
	case resp.Status == responseStatusPartial:
		reason := resp.IncompleteReason
		if reason == "" {
			reason = "unknown"
		}
		return domain.ReceiptReading{}, failurePtr(domain.AnalysisFailed("OpenAIレスポンスが未完了です: " + reason))
	case resp.Status != responseStatusDone:
		return domain.ReceiptReading{}, failurePtr(domain.AnalysisFailed("OpenAIレスポンスの状態が不正です"))
	case resp.Refusal != "":
		return domain.ReceiptReading{}, failurePtr(domain.AnalysisFailed("OpenAIが解析を拒否しました: " + resp.Refusal))
	case resp.OutputText == "":
		return domain.ReceiptReading{}, failurePtr(domain.AnalysisFailed("OpenAIレスポンスに解析結果がありません"))
	}
	var output receiptOutput
	if err := json.Unmarshal([]byte(resp.OutputText), &output); err != nil {
		return domain.ReceiptReading{}, failurePtr(domain.AnalysisFailed("解析結果がJSON Schemaに適合しません"))
	}
	return output.toReading(), nil
}

func failurePtr(reason domain.FailureReason) *domain.FailureReason { return &reason }

// categoryEnum は JSON Schema の category に許可する値。ユビキタス言語のカテゴリ(social を含む)と同じ語彙。
func categoryEnum() []string {
	categories := common.Categories()
	values := make([]string, 0, len(categories))
	for _, c := range categories {
		values = append(values, c.String())
	}
	return values
}

// ReceiptSchema は構造化出力に要求する JSON Schema。receiptOutput と同じ形。
func ReceiptSchema() map[string]any {
	detail := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "amount", "quantity", "category"}, "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "amount": map[string]any{"type": "integer"}, "quantity": map[string]any{"type": "integer"}, "category": map[string]any{"type": "string", "enum": categoryEnum()},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"store_name", "purchase_date", "total_amount", "details"}, "properties": map[string]any{
		"store_name": map[string]any{"type": []string{"string", "null"}},
		"purchase_date": map[string]any{"anyOf": []any{
			map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`, "description": "購入日。時刻を含めないYYYY-MM-DD形式"},
			map[string]any{"type": "null"},
		}},
		"total_amount": map[string]any{"type": []string{"integer", "null"}}, "details": map[string]any{"type": "array", "items": detail},
	}}
}
