package settings

import "snap_kakeibo/backend/internal/library/oswrapper"

// RetryAnalysisConfig は retry-analysis に必要な設定値。upload と違いバケットは使わず、analyze キューの URL が要る。
type RetryAnalysisConfig struct {
	AnalysisRequestsTable string
	// AnalyzeQueueURL は再解析ジョブを送る analyze キュー。
	AnalyzeQueueURL string
	// JWTSecretParameter は access token の検証鍵を持つ SSM SecureString パラメータ名。署名鍵自体は環境変数で受け取らない。
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// LoadRetryAnalysis は retry-analysis の必須設定を起動時に検証する。
func LoadRetryAnalysis(osw oswrapper.Interface) (RetryAnalysisConfig, error) {
	var cfg RetryAnalysisConfig
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"ANALYSIS_REQUESTS_TABLE", &cfg.AnalysisRequestsTable},
		{"ANALYZE_QUEUE_URL", &cfg.AnalyzeQueueURL},
		{"SSM_JWT_SECRET", &cfg.JWTSecretParameter},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return RetryAnalysisConfig{}, err
		}
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
