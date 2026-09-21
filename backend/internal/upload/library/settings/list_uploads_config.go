package settings

import "snap_kakeibo/backend/internal/library/oswrapper"

// ListUploadsConfig は list-uploads に必要な設定値。読み取りだけなのでバケットもキューも使わない。
type ListUploadsConfig struct {
	UploadHistoriesTable string
	// JWTSecretParameter は access token の検証鍵を持つ SSM SecureString パラメータ名。署名鍵自体は環境変数で受け取らない。
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// LoadListUploads は list-uploads の必須設定を起動時に検証する。
func LoadListUploads(osw oswrapper.Interface) (ListUploadsConfig, error) {
	var cfg ListUploadsConfig
	var err error
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"UPLOAD_HISTORIES_TABLE", &cfg.UploadHistoriesTable},
		{"SSM_JWT_SECRET", &cfg.JWTSecretParameter},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return ListUploadsConfig{}, err
		}
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
