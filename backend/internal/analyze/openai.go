package analyze

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const Instructions = `レシート画像から店名・購入日・合計金額・明細を読み取り、明細を固定カテゴリに分類する。読めない項目はnullにし、推測で埋めない。金額は税込の整数（円）。合計金額は「合計」「お買上げ計」など支払額の行を使う。明細は商品行のみとし、小計・税・割引・預り金・お釣りは含めない。レシートでない画像ならdetailsを空にする。`

type Client struct {
	HTTP                           *http.Client
	APIKey, Model, ReasoningEffort string
}

func (c Client) Analyze(ctx context.Context, jpegData []byte) ([]byte, error) {
	body := map[string]any{
		"model": c.Model, "store": false, "max_output_tokens": 4096,
		"reasoning":    map[string]any{"effort": c.ReasoningEffort},
		"instructions": Instructions,
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_image", "image_url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegData), "detail": "high"},
				},
			},
		},
		"text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "receipt", "strict": true, "schema": receiptSchema()}},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}
	var last error
	var lastRaw []byte
	for delay := time.Second; ; delay *= 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
			resp.Body.Close()
			if readErr != nil {
				err = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				var state struct {
					Status string `json:"status"`
				}
				if json.Unmarshal(raw, &state) == nil && state.Status == "failed" {
					lastRaw = raw
					err = fmt.Errorf("OpenAI response status failed")
				} else {
					return raw, nil
				}
			} else if resp.StatusCode != 429 && resp.StatusCode != 500 && resp.StatusCode != 502 && resp.StatusCode != 503 && resp.StatusCode != 504 {
				return raw, &Failure{"ANALYSIS_FAILED", fmt.Sprintf("OpenAI APIがHTTP %dを返しました", resp.StatusCode)}
			} else {
				err = fmt.Errorf("OpenAI temporary HTTP status %d", resp.StatusCode)
			}
		}
		last = err
		if delay > 16*time.Second {
			delay = 16 * time.Second
		}
		select {
		case <-ctx.Done():
			_ = lastRaw // failed responses are intentionally not persisted until a terminal response is received
			return nil, fmt.Errorf("%w: %v", ErrTemporary, last)
		case <-time.After(delay):
		}
	}
}

func receiptSchema() map[string]any {
	detail := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "amount", "quantity", "category"}, "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "amount": map[string]any{"type": "integer"}, "quantity": map[string]any{"type": "integer"}, "category": map[string]any{"type": "string", "enum": Categories},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"store_name", "purchased_at", "total_amount", "details"}, "properties": map[string]any{
		"store_name": map[string]any{"type": []string{"string", "null"}}, "purchased_at": map[string]any{"type": []string{"string", "null"}}, "total_amount": map[string]any{"type": []string{"integer", "null"}}, "details": map[string]any{"type": "array", "items": detail},
	}}
}
