package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validRetryAnalysisEnv() map[string]string {
	return map[string]string{
		"ANALYSIS_REQUESTS_TABLE": "analysis-requests",
		"ANALYZE_QUEUE_URL":       "http://q/analyze",
		"SSM_JWT_SECRET":          "/dev/jwt",
		"STAGE":                   "dev",
	}
}

func TestLoadRetryAnalysisReadsRequiredAndOptionalValues(t *testing.T) {
	t.Parallel()
	env := validRetryAnalysisEnv()
	env["LOG_LEVEL"] = "debug"
	cfg, err := LoadRetryAnalysis(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("LoadRetryAnalysis() error = %v", err)
	}
	want := RetryAnalysisConfig{AnalysisRequestsTable: "analysis-requests", AnalyzeQueueURL: "http://q/analyze", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadRetryAnalysisRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "ANALYSIS_REQUESTS_TABLE") }},
		{"missing queue url", func(env map[string]string) { delete(env, "ANALYZE_QUEUE_URL") }},
		{"missing jwt secret parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validRetryAnalysisEnv()
			tt.arrange(env)
			if _, err := LoadRetryAnalysis(oswrappertest.New(env)); err == nil {
				t.Fatal("LoadRetryAnalysis() error = nil, want error")
			}
		})
	}
}
