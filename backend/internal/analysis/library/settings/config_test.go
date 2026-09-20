package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validEnv() map[string]string {
	return map[string]string{
		"UPLOAD_HISTORIES_TABLE":  "upload-histories",
		"BILLINGS_TABLE":          "billings",
		"BILLING_DETAILS_TABLE":   "billing-details",
		"MONTHLY_SUMMARIES_TABLE": "monthly-summaries",
		"RECEIPT_BUCKET":          "receipts",
		"OPENAI_MODEL":            "gpt-5-mini",
		"OPENAI_REASONING_EFFORT": "low",
		"SSM_OPENAI_API_KEY":      "/dev/openai",
		"STAGE":                   "dev",
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := Load(oswrappertest.New(validEnv()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ImageMaxEdge != 2048 || cfg.LogLevel != "" || cfg.OpenAIAPIKey != "" || cfg.OpenAIAPIKeyParameter != "/dev/openai" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadReadsOptionalValues(t *testing.T) {
	t.Parallel()
	env := validEnv()
	delete(env, "SSM_OPENAI_API_KEY")
	env["OPENAI_API_KEY"] = "sk-local"
	env["IMAGE_MAX_EDGE"] = "1024"
	env["LOG_LEVEL"] = "debug"
	cfg, err := Load(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ImageMaxEdge != 1024 || cfg.LogLevel != "debug" || cfg.OpenAIAPIKey != "sk-local" || cfg.OpenAIAPIKeyParameter != "" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadRejectsMissingOrInvalidValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "BILLINGS_TABLE") }},
		{"missing bucket", func(env map[string]string) { delete(env, "RECEIPT_BUCKET") }},
		{"missing model", func(env map[string]string) { delete(env, "OPENAI_MODEL") }},
		{"missing api key and parameter", func(env map[string]string) { delete(env, "SSM_OPENAI_API_KEY") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
		{"invalid image max edge", func(env map[string]string) { env["IMAGE_MAX_EDGE"] = "large" }},
		{"non-positive image max edge", func(env map[string]string) { env["IMAGE_MAX_EDGE"] = "0" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			tt.arrange(env)
			if _, err := Load(oswrappertest.New(env)); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}
