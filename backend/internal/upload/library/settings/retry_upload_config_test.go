package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validRetryUploadEnv() map[string]string {
	return map[string]string{
		"UPLOAD_HISTORIES_TABLE": "upload-histories",
		"ANALYZE_QUEUE_URL":      "http://q/analyze",
		"SSM_JWT_SECRET":         "/dev/jwt",
		"STAGE":                  "dev",
	}
}

func TestLoadRetryUploadReadsRequiredAndOptionalValues(t *testing.T) {
	t.Parallel()
	env := validRetryUploadEnv()
	env["LOG_LEVEL"] = "debug"
	cfg, err := LoadRetryUpload(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("LoadRetryUpload() error = %v", err)
	}
	want := RetryUploadConfig{UploadHistoriesTable: "upload-histories", AnalyzeQueueURL: "http://q/analyze", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadRetryUploadRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "UPLOAD_HISTORIES_TABLE") }},
		{"missing queue url", func(env map[string]string) { delete(env, "ANALYZE_QUEUE_URL") }},
		{"missing jwt secret parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validRetryUploadEnv()
			tt.arrange(env)
			if _, err := LoadRetryUpload(oswrappertest.New(env)); err == nil {
				t.Fatal("LoadRetryUpload() error = nil, want error")
			}
		})
	}
}
