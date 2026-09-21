package settings

import "snap_kakeibo/backend/internal/library/oswrapper"

// ListAnalysisRequestsConfig は list-analysis-requests に必要な設定値。読み取りだけなのでバケットもキューも使わない。
type ListAnalysisRequestsConfig struct {
	AnalysisRequestsTable string
	// JWTSecretParameter は access token の検証鍵を持つ SSM SecureString パラメータ名。署名鍵自体は環境変数で受け取らない。
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// LoadListAnalysisRequests は list-analysis-requests の必須設定を起動時に検証する。
func LoadListAnalysisRequests(osw oswrapper.Interface) (ListAnalysisRequestsConfig, error) {
	var cfg ListAnalysisRequestsConfig
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"ANALYSIS_REQUESTS_TABLE", &cfg.AnalysisRequestsTable},
		{"SSM_JWT_SECRET", &cfg.JWTSecretParameter},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return ListAnalysisRequestsConfig{}, err
		}
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
