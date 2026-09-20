// Package settings は認証 Lambda の起動時設定を検証して読み込む。
package settings

import (
	"errors"
	"fmt"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

// LoginConfig は auth-login に必要な設定値。
type LoginConfig struct {
	UsersTable         string
	JWTSecret          string
	JWTSecretParameter string
	Stage              string
	LogLevel           string
}

// LoadLoginConfig は必須設定を起動時に検証する。
func LoadLoginConfig(osw oswrapper.Interface) (LoginConfig, error) {
	usersTable, err := osw.GetEnv("USERS_TABLE")
	if err != nil {
		return LoginConfig{}, err
	}
	jwtSecret, jwtSecretErr := osw.GetEnv("JWT_SECRET")
	jwtSecretParameter, parameterErr := osw.GetEnv("SSM_JWT_SECRET")
	if jwtSecretErr != nil && parameterErr != nil {
		return LoginConfig{}, fmt.Errorf("JWT_SECRET or SSM_JWT_SECRET is required: %w", errors.Join(jwtSecretErr, parameterErr))
	}
	stage, err := osw.GetEnv("STAGE")
	if err != nil {
		return LoginConfig{}, err
	}
	logLevel, _ := osw.GetEnv("LOG_LEVEL")
	return LoginConfig{UsersTable: usersTable, JWTSecret: jwtSecret, JWTSecretParameter: jwtSecretParameter, Stage: stage, LogLevel: logLevel}, nil
}
