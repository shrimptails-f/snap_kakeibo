// Package settings は upload Lambda の起動時設定を検証して読み込む。
package settings

import (
	"errors"
	"fmt"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

// Config は upload に必要な設定値。
type Config struct {
	UploadHistoriesTable string
	// ReceiptBucket は元画像を PUT するバケット。
	ReceiptBucket string
	// JWTSecret / JWTSecretParameter は access token の検証鍵。どちらか一方が必須で、JWTSecret を優先する。
	JWTSecret          string
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
		{"UPLOAD_HISTORIES_TABLE", &cfg.UploadHistoriesTable},
		{"RECEIPT_BUCKET", &cfg.ReceiptBucket},
		{"STAGE", &cfg.Stage},
	} {
		if *v.dst, err = osw.GetEnv(v.key); err != nil {
			return Config{}, err
		}
	}
	var jwtSecretErr, parameterErr error
	cfg.JWTSecret, jwtSecretErr = osw.GetEnv("JWT_SECRET")
	cfg.JWTSecretParameter, parameterErr = osw.GetEnv("SSM_JWT_SECRET")
	if jwtSecretErr != nil && parameterErr != nil {
		return Config{}, fmt.Errorf("JWT_SECRET or SSM_JWT_SECRET is required: %w", errors.Join(jwtSecretErr, parameterErr))
	}
	cfg.LogLevel, _ = osw.GetEnv("LOG_LEVEL")
	return cfg, nil
}
