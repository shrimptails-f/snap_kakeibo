package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validEnv() map[string]string {
	return map[string]string{
		"UPLOAD_HISTORIES_TABLE": "upload-histories",
		"RECEIPT_BUCKET":         "receipts",
		"SSM_JWT_SECRET":         "/dev/jwt",
		"STAGE":                  "dev",
	}
}

func TestLoadReadsRequiredAndOptionalValues(t *testing.T) {
	t.Parallel()
	env := validEnv()
	env["LOG_LEVEL"] = "debug"
	cfg, err := Load(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{UploadHistoriesTable: "upload-histories", ReceiptBucket: "receipts", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadAcceptsStaticJWTSecretWithoutParameter(t *testing.T) {
	t.Parallel()
	env := validEnv()
	delete(env, "SSM_JWT_SECRET")
	env["JWT_SECRET"] = "secret"
	cfg, err := Load(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.JWTSecret != "secret" || cfg.JWTSecretParameter != "" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "UPLOAD_HISTORIES_TABLE") }},
		{"missing bucket", func(env map[string]string) { delete(env, "RECEIPT_BUCKET") }},
		{"missing jwt secret and parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
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
