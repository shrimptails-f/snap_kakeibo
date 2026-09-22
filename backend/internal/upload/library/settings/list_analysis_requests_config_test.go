package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validListAnalysisRequestsEnv() map[string]string {
	return map[string]string{
		"ANALYSIS_REQUESTS_TABLE": "analysis-requests",
		"EXPENSES_TABLE":          "expenses",
		"SSM_JWT_SECRET":          "/dev/jwt",
		"STAGE":                   "dev",
	}
}

func TestLoadListAnalysisRequestsReadsRequiredAndOptionalValues(t *testing.T) {
	t.Parallel()
	env := validListAnalysisRequestsEnv()
	env["LOG_LEVEL"] = "debug"
	cfg, err := LoadListAnalysisRequests(oswrappertest.New(env))
	if err != nil {
		t.Fatalf("LoadListAnalysisRequests() error = %v", err)
	}
	want := ListAnalysisRequestsConfig{AnalysisRequestsTable: "analysis-requests", ExpensesTable: "expenses", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadListAnalysisRequestsRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing table", func(env map[string]string) { delete(env, "ANALYSIS_REQUESTS_TABLE") }},
		{"missing expenses table", func(env map[string]string) { delete(env, "EXPENSES_TABLE") }},
		{"missing jwt secret parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
		{"missing stage", func(env map[string]string) { delete(env, "STAGE") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := validListAnalysisRequestsEnv()
			tt.arrange(env)
			if _, err := LoadListAnalysisRequests(oswrappertest.New(env)); err == nil {
				t.Fatal("LoadListAnalysisRequests() error = nil, want error")
			}
		})
	}
}
