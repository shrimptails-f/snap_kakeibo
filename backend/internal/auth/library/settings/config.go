// Package settings は認証 Lambda の起動時設定を検証して読み込む。
package settings

import (
	"errors"
	"fmt"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

// Config はトークンを発行する認証 Lambda（auth-login / auth-refresh）に必要な設定値。
type Config struct {
	UsersTable         string
	RefreshTokensTable string
	JWTSecret          string
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// Load は必須設定を起動時に検証する。
func Load(osw oswrapper.Interface) (Config, error) {
	usersTable, err := osw.GetEnv("USERS_TABLE")
	if err != nil {
		return Config{}, err
	}
	refreshTokensTable, err := osw.GetEnv("REFRESH_TOKENS_TABLE")
	if err != nil {
		return Config{}, err
	}
	jwtSecret, jwtSecretErr := osw.GetEnv("JWT_SECRET")
	jwtSecretParameter, parameterErr := osw.GetEnv("SSM_JWT_SECRET")
	if jwtSecretErr != nil && parameterErr != nil {
		return Config{}, fmt.Errorf("JWT_SECRET or SSM_JWT_SECRET is required: %w", errors.Join(jwtSecretErr, parameterErr))
	}
	stage, err := osw.GetEnv("STAGE")
	if err != nil {
		return Config{}, err
	}
	logLevel, _ := osw.GetEnv("LOG_LEVEL")
	return Config{UsersTable: usersTable, RefreshTokensTable: refreshTokensTable, JWTSecret: jwtSecret, JWTSecretParameter: jwtSecretParameter, Stage: stage, LogLevel: logLevel}, nil
}

// LoadLogout は auth-logout に必要な設定だけを起動時に検証して読み込む。
func LoadLogout(osw oswrapper.Interface) (Config, error) {
	refreshTokensTable, err := osw.GetEnv("REFRESH_TOKENS_TABLE")
	if err != nil {
		return Config{}, err
	}
	stage, err := osw.GetEnv("STAGE")
	if err != nil {
		return Config{}, err
	}
	logLevel, _ := osw.GetEnv("LOG_LEVEL")
	return Config{RefreshTokensTable: refreshTokensTable, Stage: stage, LogLevel: logLevel}, nil
}
