// Package settings は get-expense Lambda の起動時設定を検証して読み込む。
package settings

import "snap_kakeibo/backend/internal/library/oswrapper"

// Config は get-expense に必要な設定値。
type Config struct {
	ExpensesTable         string
	ExpenseDetailsTable   string
	MonthlySummariesTable string
	// JWTSecretParameter は access token の検証鍵を持つ SSM SecureString パラメータ名。署名鍵自体は環境変数で受け取らない。
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// Load は必須設定を起動時に検証する。
func Load(osw oswrapper.Interface) (Config, error) {
	var cfg Config
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"EXPENSES_TABLE", &cfg.ExpensesTable},
		{"EXPENSE_DETAILS_TABLE", &cfg.ExpenseDetailsTable},
		{"MONTHLY_SUMMARIES_TABLE", &cfg.MonthlySummariesTable},
		{"SSM_JWT_SECRET", &cfg.JWTSecretParameter},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return Config{}, err
		}
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
