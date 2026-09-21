package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validListUploadsEnv() map[string]string {
	return map[string]string{
		"UPLOAD_HISTORIES_TABLE": "upload-histories",
		"SSM_JWT_SECRET":         "/dev/jwt",
		"STAGE":                  "dev",
	}
}

func TestLoadListUploadsReadsRequiredAndOptionalValues(t *testing.T) {
	t.Parallel()
	env := validListUploadsEnv()
	env["LOG_LEVEL"] = "debug"
	cfg, err := LoadListUploads(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("LoadListUploads() error = %v", err)
	}
	want := ListUploadsConfig{UploadHistoriesTable: "upload-histories", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadListUploadsRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "UPLOAD_HISTORIES_TABLE") }},
		{"missing jwt secret parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validListUploadsEnv()
			tt.arrange(env)
			if _, err := LoadListUploads(oswrappertest.New(env)); err == nil {
				t.Fatal("LoadListUploads() error = nil, want error")
			}
		})
	}
}
