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
	TaxMarks         []domain.TaxMark         `json:"tax_marks"`
	TaxBreakdown     []domain.TaxBreakdown    `json:"tax_breakdown"`
	Details          []detailOutput           `json:"details"`
}

type detailOutput struct {
	Name           string `json:"name"`
	Amount         int64  `json:"amount"`
	Quantity       int64  `json:"quantity"`
	Category       string `json:"category"`
	TaxRate        *int64 `json:"tax_rate"`
	TaxMode        string `json:"tax_mode"`
	TaxMark        string `json:"tax_mark"`
	DiscountAmount *int64 `json:"discount_amount"`
}

func (o receiptOutput) toReading() domain.ReceiptReading {
	details := make([]domain.ReadDetail, 0, len(o.Details))
	for _, d := range o.Details {
		details = append(details, domain.ReadDetail{Name: d.Name, Amount: d.Amount, Quantity: d.Quantity, Category: d.Category, TaxRate: d.TaxRate, TaxMode: d.TaxMode, TaxMark: d.TaxMark, DiscountAmount: d.DiscountAmount})
	}
	return domain.ReceiptReading{StoreName: o.StoreName, PurchaseDate: o.PurchaseDate, AmountCandidates: o.AmountCandidates, TaxBreakdown: o.TaxBreakdown, TaxMarks: o.TaxMarks, Details: details}
}

// Instructions は OpenAI に渡すプロンプト。
const Instructions = `レシート画像を上から下まで確認し、印字された事実をJSONに転記してください。金額を辻褄合わせで書き換えず、印字のない値はnullにしてください。

【商品明細 details】
・商品を省略せず、最後の商品（少額の袋などを含む）まで1商品につき1行を作成してください。商品名と価格が別行でも同じ商品として対応させてください。
・amountは数量反映後の行金額です。「単価×数量」の単価を行金額と取り違えず、同じ商品を二重登録しないでください。quantityには数量を記録します。値引行・小計・税・支払いは商品ではありません。
・価格の右側の「軽」「※」「C」「V」などを金額の数字と混同せず、tax_markに原文のまま残してください。印がなければ空文字です。
・discount_amountは、その印字行金額からさらに引くことが明示された商品別値引額（正の整数円）です。既に値引き反映済みなら0、対応不明ならnullです。
・tax_rateは商品に明示された税率か、印とレシートの注記から分かる税率だけを記録します。商品名・カテゴリから税率を推測しないでください。印がないことを10%の根拠にしないでください。tax_modeはincluded（内税）/ external（外税）/ mixed / unknownです。amountは税込換算しません。
・nameは商品名、categoryは固定カテゴリから分類します。

【印と注記 tax_marks】
商品行の印の意味を示すレシート末尾などの注記を、mark（印）、rate（税率）、note（注記原文）として記録してください。「軽」は軽減税率対象と注記され、8%対象額も印字されている場合は8%の対応を記録できます。対応が読めなければrateはnullです。

【金額候補 amount_candidates】
商品以外の金額行を印字ラベルlabel、上からの順番position、役割roleとともに列挙してください。最終支払合計final_total、小計・商品合計subtotal、個別値引きと値引合計discount、税額tax、税率別対象額taxable、預りdeposit、釣銭change、決済額payment、不明unknownです。商品価格や単価はdetailsへ記録し、ここに重複させません。見出しが別行なら直前の見出しと税率をlabelに含めます。合計・商品合計・対象額・決済額が別行なら同じ金額でも残します。

【税率別内訳 tax_breakdown】
印字のある税率ごとにrate、taxable_amount（税抜対象額）、gross_amount（税込対象額）、tax_amount（税額）を記録してください。税抜対象額がない場合、税込対象額から逆算せずtaxable_amountはnullです。対象額や税額が印字されない税率の行を追加してはいけません。
modeは商品行が税抜で最後に加税するexternal、商品行が税込で税額は内訳のincluded、不明ならunknownです。「内消費税」の文字だけで商品行の方式を決めず、商品合計・値引き・支払合計との関係も確認してください。

【最終確認】
商品名・金額の対応、全商品行と数量、価格の右の小さい印、税率別の見出し、値引き、桁を画像と再照合してください。合計に矛盾があれば印字を見直し、数字を計算で補ってはいけません。
store_nameは店名、purchase_dateはYYYY-MM-DD。レシート以外ならdetailsを空にしてください。`

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
	integer := map[string]any{"type": "integer"}
	text := map[string]any{"type": "string"}
	nullableInteger := map[string]any{"type": []string{"integer", "null"}}
	modes := map[string]any{"type": "string", "enum": []string{"included", "external", "unknown"}}
	detail := receiptObject([]string{"name", "amount", "quantity", "category", "tax_rate", "tax_mode", "tax_mark", "discount_amount"}, map[string]any{
		"name": text, "amount": integer, "quantity": integer, "category": map[string]any{"type": "string", "enum": categoryEnum()},
		"tax_rate": nullableInteger, "tax_mode": map[string]any{"type": "string", "enum": []string{"included", "external", "mixed", "unknown"}},
		"tax_mark": text, "discount_amount": nullableInteger,
	})
	candidate := receiptObject([]string{"amount", "label", "role", "position"}, map[string]any{
		"amount": integer, "label": text, "position": integer,
		"role": map[string]any{"type": "string", "enum": []string{"final_total", "subtotal", "discount", "tax", "taxable", "deposit", "change", "payment", "unknown"}},
	})
	tax := receiptObject([]string{"rate", "taxable_amount", "gross_amount", "tax_amount", "mode"}, map[string]any{
		"rate": nullableInteger, "taxable_amount": nullableInteger, "gross_amount": nullableInteger, "tax_amount": nullableInteger, "mode": modes,
	})
	mark := receiptObject([]string{"mark", "rate", "note"}, map[string]any{"mark": text, "rate": nullableInteger, "note": text})
	return receiptObject([]string{"store_name", "purchase_date", "amount_candidates", "tax_breakdown", "tax_marks", "details"}, map[string]any{
		"store_name": map[string]any{"type": []string{"string", "null"}},
		"purchase_date": map[string]any{"anyOf": []any{
			map[string]any{"type": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`, "description": "購入日。時刻を含めないYYYY-MM-DD形式"}, map[string]any{"type": "null"},
		}},
		"amount_candidates": map[string]any{"type": "array", "items": candidate}, "tax_breakdown": map[string]any{"type": "array", "items": tax},
		"tax_marks": map[string]any{"type": "array", "items": mark}, "details": map[string]any{"type": "array", "items": detail},
	})
}

func receiptObject(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}
