package settings

import (
	"testing"

	"snap_kakeibo/backend/internal/library/oswrapper/oswrappertest"
)

func validEnv() map[string]string {
	return map[string]string{
		"EXPENSES_TABLE":        "expenses",
		"EXPENSE_DETAILS_TABLE": "expense-details",
		"SSM_JWT_SECRET":        "/dev/jwt",
		"STAGE":                 "dev",
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
	want := Config{ExpensesTable: "expenses", ExpenseDetailsTable: "expense-details", JWTSecretParameter: "/dev/jwt", Stage: "dev", LogLevel: "debug"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		arrange func(env map[string]string)
	}{
		{"missing expenses table", func(env map[string]string) { delete(env, "EXPENSES_TABLE") }},
		{"missing details table", func(env map[string]string) { delete(env, "EXPENSE_DETAILS_TABLE") }},
		{"missing jwt secret parameter", func(env map[string]string) { delete(env, "SSM_JWT_SECRET") }},
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
