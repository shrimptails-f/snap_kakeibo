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
	StoreName        *string                  `json:"store_name"`
	PurchaseDate     *string                  `json:"purchase_date"`
	AmountCandidates []domain.AmountCandidate `json:"amount_candidates"`
	TaxBreakdown     []domain.TaxBreakdown    `json:"tax_breakdown"`
	Details          []detailOutput           `json:"details"`
}

type detailOutput struct {
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Quantity int64  `json:"quantity"`
	Category string `json:"category"`
	TaxRate  *int64 `json:"tax_rate"`
	TaxMode  string `json:"tax_mode"`
}

func (o receiptOutput) toReading() domain.ReceiptReading {
	details := make([]domain.ReadDetail, 0, len(o.Details))
	for _, d := range o.Details {
		details = append(details, domain.ReadDetail{Name: d.Name, Amount: d.Amount, Quantity: d.Quantity, Category: d.Category, TaxRate: d.TaxRate, TaxMode: d.TaxMode})
	}
	return domain.ReceiptReading{StoreName: o.StoreName, PurchaseDate: o.PurchaseDate, AmountCandidates: o.AmountCandidates, TaxBreakdown: o.TaxBreakdown, Details: details}
}

// Instructions は OpenAI に渡すプロンプト。
const Instructions = `レシート画像から店名・購入日・商品行・印字された金額行・税率別内訳を読み取る。読めない値はnullにし、推測で補わない。購入日はYYYY-MM-DD。金額は印字額の整数円で、商品行を根拠なく税込みに換算しない。amount_candidatesには最終支払合計、商品代金・商品合計、小計、個別値引きと値引合計、税額、税率別対象額、預り金、お釣り、支払方法別の決済額を印字ラベル・上からの順番(position)・役割(role)とともに列挙する。金額行の見出しが別行なら直前の見出しもlabelに含め、役割の判定に使う。最終支払額の候補も複数あればすべて残す。roleはfinal_total,subtotal,discount,tax,taxable,deposit,change,payment,unknownから選ぶ。tax_breakdownには8%・10%など税率ごとの税抜対象額と税額、商品行が税抜で最後に税を加える外税(external)か、税込の商品行に税を内訳表示する内税(included)か不明(unknown)かを記す。税抜小計と合計後の税込対象額が両方ある場合、taxable_amountには税抜小計を入れ、税込対象額はamount_candidatesに別行で残す。税込対象額だけが印字され税抜対象額がなければtaxable_amountはnullにする。「内消費税」と書かれた合計後の内訳だけで商品行を内税と決めない。detailsは商品行のみで、amountは印字された行金額、税率と内外税が分かる場合だけ記す。明細を固定カテゴリに分類する。レシート以外ならdetailsを空にする。`

const (
	// DefaultRequestTimeout は 1 ジョブあたりの OpenAI 呼び出し(リトライ込み)の上限。
	// 5 分の試行 2 回と 1 秒の待機を収め、Lambda の 11 分上限より短くする
	DefaultRequestTimeout = 10*time.Minute + 30*time.Second
	maxOutputTokens       = 8192
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
	detail := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "amount", "quantity", "category", "tax_rate", "tax_mode"}, "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "amount": map[string]any{"type": "integer"}, "quantity": map[string]any{"type": "integer"}, "category": map[string]any{"type": "string", "enum": categoryEnum()}, "tax_rate": map[string]any{"type": []string{"integer", "null"}}, "tax_mode": map[string]any{"type": "string", "enum": []string{"included", "external", "mixed", "unknown"}},
	}}
	candidate := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"amount", "label", "role", "position"}, "properties": map[string]any{"amount": map[string]any{"type": "integer"}, "label": map[string]any{"type": "string"}, "role": map[string]any{"type": "string", "enum": []string{"final_total", "subtotal", "discount", "tax", "taxable", "deposit", "change", "payment", "unknown"}}, "position": map[string]any{"type": "integer"}}}
	tax := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"rate", "taxable_amount", "tax_amount", "mode"}, "properties": map[string]any{"rate": map[string]any{"type": []string{"integer", "null"}}, "taxable_amount": map[string]any{"type": []string{"integer", "null"}}, "tax_amount": map[string]any{"type": []string{"integer", "null"}}, "mode": map[string]any{"type": "string", "enum": []string{"included", "external", "unknown"}}}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"store_name", "purchase_date", "amount_candidates", "tax_breakdown", "details"}, "properties": map[string]any{
		"store_name": map[string]any{"type": []string{"string", "null"}},
		"purchase_date": map[string]any{"anyOf": []any{
			map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`, "description": "購入日。時刻を含めないYYYY-MM-DD形式"},
			map[string]any{"type": "null"},
		}},
		"amount_candidates": map[string]any{"type": "array", "items": candidate}, "tax_breakdown": map[string]any{"type": "array", "items": tax}, "details": map[string]any{"type": "array", "items": detail},
	}}
}
