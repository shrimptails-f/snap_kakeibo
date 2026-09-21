package settings

import "snap_kakeibo/backend/internal/library/oswrapper"

// RetryUploadConfig は retry-upload に必要な設定値。upload と違いバケットは使わず、analyze キューの URL が要る。
type RetryUploadConfig struct {
	UploadHistoriesTable string
	// AnalyzeQueueURL は再実行ジョブを送る analyze キュー。
	AnalyzeQueueURL string
	// JWTSecretParameter は access token の検証鍵を持つ SSM SecureString パラメータ名。署名鍵自体は環境変数で受け取らない。
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// LoadRetryUpload は retry-upload の必須設定を起動時に検証する。
func LoadRetryUpload(osw oswrapper.Interface) (RetryUploadConfig, error) {
	var cfg RetryUploadConfig
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"UPLOAD_HISTORIES_TABLE", &cfg.UploadHistoriesTable},
		{"ANALYZE_QUEUE_URL", &cfg.AnalyzeQueueURL},
		{"SSM_JWT_SECRET", &cfg.JWTSecretParameter},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return RetryUploadConfig{}, err
		}
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
