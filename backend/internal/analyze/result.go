package analyze

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var Categories = []string{"food", "daily_goods", "medical", "transport", "utilities", "entertainment", "clothing", "education", "other", "unknown"}

type Receipt struct {
	StoreName   *string  `json:"store_name"`
	PurchasedAt *string  `json:"purchased_at"`
	TotalAmount *int64   `json:"total_amount"`
	Details     []Detail `json:"details"`
}

type Detail struct {
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Quantity int64  `json:"quantity"`
	Category string `json:"category"`
}

type Failure struct{ Code, Message string }

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

func Validate(r Receipt, now time.Time) (Receipt, *Failure) {
	if r.PurchasedAt == nil {
		return r, &Failure{"NO_DATE", "購入日を取得できませんでした"}
	}
	d, err := time.ParseInLocation("2006-01-02", *r.PurchasedAt, now.Location())
	if err != nil || d.After(dateOnly(now).AddDate(0, 0, 1)) || d.Before(dateOnly(now).AddDate(-5, 0, 0)) {
		return r, &Failure{"INVALID_DATE", "購入日が有効な範囲ではありません"}
	}
	if r.TotalAmount == nil {
		return r, &Failure{"NO_TOTAL_AMOUNT", "合計金額を取得できませんでした"}
	}
	if *r.TotalAmount < 1 || *r.TotalAmount > 10_000_000 {
		return r, &Failure{"INVALID_AMOUNT", "合計金額が有効な範囲ではありません"}
	}
	if len(r.Details) > 50 {
		return r, &Failure{"TOO_MANY_DETAILS", "明細件数が50件を超えています"}
	}
	if r.StoreName != nil {
		s := truncate(*r.StoreName, 100)
		r.StoreName = &s
	}
	valid := make([]Detail, 0, len(r.Details))
	for _, d := range r.Details {
		if d.Amount < 0 || d.Amount > 10_000_000 || d.Quantity < 1 || d.Quantity > 999 {
			return r, &Failure{"INVALID_AMOUNT", "明細の金額または数量が有効な範囲ではありません"}
		}
		d.Name = truncate(strings.TrimSpace(d.Name), 100)
		if d.Name == "" {
			continue
		}
		if !validCategory(d.Category) {
			d.Category = "unknown"
		}
		valid = append(valid, d)
	}
	r.Details = valid
	return r, nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}
func validCategory(s string) bool {
	for _, c := range Categories {
		if s == c {
			return true
		}
	}
	return false
}

type APIResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		Type    string                                 `json:"type"`
		Content []struct{ Type, Text, Refusal string } `json:"content"`
	} `json:"output"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error json.RawMessage `json:"error"`
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

var ErrTemporary = errors.New("temporary OpenAI failure")

func ParseResponse(raw []byte) (Receipt, APIResponse, error) {
	var resp APIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "OpenAIレスポンスを解析できませんでした"}
	}
	if resp.Status == "failed" {
		return Receipt{}, resp, fmt.Errorf("%w: response status failed", ErrTemporary)
	}
	if resp.Status == "incomplete" {
		reason := "unknown"
		if resp.IncompleteDetails != nil {
			reason = resp.IncompleteDetails.Reason
		}
		return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "OpenAIレスポンスが未完了です: " + reason}
	}
	if resp.Status != "completed" {
		return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "OpenAIレスポンスの状態が不正です"}
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Type == "refusal" {
				return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "OpenAIが解析を拒否しました: " + c.Refusal}
			}
			if c.Type == "output_text" && c.Text != "" {
				var r Receipt
				if err := json.Unmarshal([]byte(c.Text), &r); err != nil {
					return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "解析結果がJSON Schemaに適合しません"}
				}
				return r, resp, nil
			}
		}
	}
	return Receipt{}, resp, &Failure{"ANALYSIS_FAILED", "OpenAIレスポンスに解析結果がありません"}
}
