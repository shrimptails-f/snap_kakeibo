// Package settings は analyze-receipt Lambda の起動時設定を検証して読み込む。
package settings

import (
	"errors"
	"fmt"
	"strconv"

	"snap_kakeibo/backend/internal/analysis/library/image"
	"snap_kakeibo/backend/internal/library/oswrapper"
)

// Config は analyze-receipt に必要な設定値。
type Config struct {
	UploadHistoriesTable  string
	BillingsTable         string
	BillingDetailsTable   string
	MonthlySummariesTable string
	// ReceiptBucket は元画像と解析結果を置くバケット。
	ReceiptBucket string
	// OpenAIAPIKey は環境変数で直接渡された API key。ローカル実行向けで、空なら OpenAIAPIKeyParameter を使う。
	OpenAIAPIKey string
	// OpenAIAPIKeyParameter は API key を持つ SSM SecureString パラメータ名。
	OpenAIAPIKeyParameter string
	OpenAIModel           string
	OpenAIReasoningEffort string
	// ImageMaxEdge は OpenAI に渡す画像の長辺の上限(px)。未設定なら image.DefaultMaxEdge。
	ImageMaxEdge int
	Stage        string
	LogLevel     string
}

// Load は必須設定を起動時に検証する。
func Load(osw oswrapper.Interface) (Config, error) {
	var cfg Config
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"UPLOAD_HISTORIES_TABLE", &cfg.UploadHistoriesTable},
		{"BILLINGS_TABLE", &cfg.BillingsTable},
		{"BILLING_DETAILS_TABLE", &cfg.BillingDetailsTable},
		{"MONTHLY_SUMMARIES_TABLE", &cfg.MonthlySummariesTable},
		{"RECEIPT_BUCKET", &cfg.ReceiptBucket},
		{"OPENAI_MODEL", &cfg.OpenAIModel},
		{"OPENAI_REASONING_EFFORT", &cfg.OpenAIReasoningEffort},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return Config{}, err
		}
	}
	apiKey, apiKeyErr := osw.GetEnv("OPENAI_API_KEY")
	parameter, parameterErr := osw.GetEnv("SSM_OPENAI_API_KEY")
	if apiKeyErr != nil && parameterErr != nil {
		return Config{}, fmt.Errorf("OPENAI_API_KEY or SSM_OPENAI_API_KEY is required: %w", errors.Join(apiKeyErr, parameterErr))
	}
	cfg.OpenAIAPIKey, cfg.OpenAIAPIKeyParameter = apiKey, parameter

	cfg.ImageMaxEdge = image.DefaultMaxEdge
	if raw, err := osw.GetEnv("IMAGE_MAX_EDGE"); err == nil {
		edge, err := strconv.Atoi(raw)
		if err != nil || edge <= 0 {
			return Config{}, fmt.Errorf("IMAGE_MAX_EDGE must be a positive integer: %q", raw)
		}
		cfg.ImageMaxEdge = edge
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
