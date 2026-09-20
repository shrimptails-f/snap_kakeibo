package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/openai"
)

// Instructions は OpenAI に渡すプロンプト。
const Instructions = `レシート画像から店名・購入日・合計金額・明細を読み取り、明細を固定カテゴリに分類する。読めない項目はnullにし、推測で埋めない。購入日は時刻を含めずYYYY-MM-DD形式にする。金額は税込の整数（円）。合計金額は「合計」「お買上げ計」など支払額の行を使う。明細は商品行のみとし、小計・税・割引・預り金・お釣りは含めない。レシートでない画像ならdetailsを空にする。`

const (
	// DefaultRequestTimeout は 1 ジョブあたりの OpenAI 呼び出し(リトライ込み)の上限。
	// Lambda のタイムアウト(3 分)より短くし、span_finished を出してから終われるようにする
	DefaultRequestTimeout = 120 * time.Second
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

// Analyze は画像を送り、応答をレシートに変換する。
// 4xx など再試行しても解決しない応答は Failure として返し、一時的な失敗は error として返す。
func (a OpenAIReceiptAnalyzer) Analyze(ctx context.Context, jpeg []byte) (application.AnalysisResult, error) {
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
			// 認証・不正なリクエストなどは何度送っても同じなので、このアップロードの失敗として記録する
			return application.AnalysisResult{Failure: &domain.Failure{Code: domain.FailureAnalysisFailed, Message: fmt.Sprintf("OpenAI APIがHTTP %dを返しました", apiErr.StatusCode)}}, nil
		}
		return application.AnalysisResult{}, err
	}
	result := application.AnalysisResult{
		ResponseID: resp.ID,
		Raw:        resp.Raw,
		Usage: application.TokenUsage{
			InputTokens:     resp.Usage.InputTokens,
			OutputTokens:    resp.Usage.OutputTokens,
			ReasoningTokens: resp.Usage.ReasoningTokens,
		},
	}
	result.Receipt, result.Failure = receiptFromResponse(resp)
	return result, nil
}

// receiptFromResponse は応答本文をレシートに変換する。採用できない応答は Failure を返す。
func receiptFromResponse(resp *openai.Response) (domain.Receipt, *domain.Failure) {
	switch {
	case resp.Status == responseStatusPartial:
		reason := resp.IncompleteReason
		if reason == "" {
			reason = "unknown"
		}
		return domain.Receipt{}, &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "OpenAIレスポンスが未完了です: " + reason}
	case resp.Status != responseStatusDone:
		return domain.Receipt{}, &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "OpenAIレスポンスの状態が不正です"}
	case resp.Refusal != "":
		return domain.Receipt{}, &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "OpenAIが解析を拒否しました: " + resp.Refusal}
	case resp.OutputText == "":
		return domain.Receipt{}, &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "OpenAIレスポンスに解析結果がありません"}
	}
	var receipt domain.Receipt
	if err := json.Unmarshal([]byte(resp.OutputText), &receipt); err != nil {
		return domain.Receipt{}, &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "解析結果がJSON Schemaに適合しません"}
	}
	return receipt, nil
}

// ReceiptSchema は構造化出力に要求する JSON Schema。domain.Receipt と同じ形。
func ReceiptSchema() map[string]any {
	detail := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "amount", "quantity", "category"}, "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "amount": map[string]any{"type": "integer"}, "quantity": map[string]any{"type": "integer"}, "category": map[string]any{"type": "string", "enum": domain.Categories},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"store_name", "purchased_at", "total_amount", "details"}, "properties": map[string]any{
		"store_name": map[string]any{"type": []string{"string", "null"}},
		"purchased_at": map[string]any{"anyOf": []any{
			map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`, "description": "購入日。時刻を含めないYYYY-MM-DD形式"},
			map[string]any{"type": "null"},
		}},
		"total_amount": map[string]any{"type": []string{"integer", "null"}}, "details": map[string]any{"type": "array", "items": detail},
	}}
}
