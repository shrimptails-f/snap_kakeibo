// Package di は Lambda ごとの依存性を合成する。
package di

import (
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/auth/library/password"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/auth/library/token"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewAuthLoginContainer は共通コンテナへ auth-login 固有の依存性を追加する。
func NewAuthLoginContainer(cfg settings.LoginConfig, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create auth-login container: %w", err)
	}
	if err := Provide(container, "auth-login",
		func() settings.LoginConfig { return cfg },
		func(client *libdynamodb.Client, cfg settings.LoginConfig) application.UserRepository {
			return infrastructure.DynamoDBUserRepository{Table: client.Table(cfg.UsersTable)}
		},
		func() application.PasswordComparator { return password.Bcrypt{} },
		func(cfg settings.LoginConfig, client *libssm.Client) token.SecretProvider {
			if cfg.JWTSecret != "" {
				return token.StaticSecretProvider{Value: cfg.JWTSecret}
			}
			return &token.SSMSecretProvider{Parameter: client.Parameter(cfg.JWTSecretParameter)}
		},
		func(secrets token.SecretProvider, cfg settings.LoginConfig, clock timewrapper.Interface) application.AccessTokenIssuer {
			return token.JWTIssuer{Secrets: secrets, Issuer: issuer(cfg.Stage), Clock: clock, TTL: 15 * time.Minute}
		},
		func() application.RefreshTokenGenerator { return token.RefreshTokenGenerator{} },
		func(client *libdynamodb.Client, cfg settings.LoginConfig) application.RefreshTokenRepository {
			return infrastructure.DynamoDBRefreshTokenRepository{Table: client.Table(cfg.UsersTable)}
		},
		func(client *libdynamodb.Client, cfg settings.LoginConfig, clock timewrapper.Interface) application.LoginAttemptLimiter {
			return infrastructure.DynamoDBLoginAttemptLimiter{Table: client.Table(cfg.UsersTable), Clock: clock}
		},
		application.NewLoginUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveLoginAttemptLimiter はログイン試行制限器をコンテナから取り出す。
func ResolveLoginAttemptLimiter(container *dig.Container) (application.LoginAttemptLimiter, error) {
	var limiter application.LoginAttemptLimiter
	if err := container.Invoke(func(resolved application.LoginAttemptLimiter) { limiter = resolved }); err != nil {
		return nil, fmt.Errorf("resolve login attempt limiter: %w", err)
	}
	return limiter, nil
}

// ResolveAuthLoginUsecase はコンテナから HTTP 層が依存するユースケース契約を取り出す。
func ResolveAuthLoginUsecase(container *dig.Container) (application.LoginUsecaseInterface, error) {
	var usecase application.LoginUsecaseInterface
	if err := container.Invoke(func(resolved application.LoginUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve auth-login usecase: %w", err)
	}
	return usecase, nil
}

func issuer(stage string) string {
	if stage == "" {
		return "snap-kakeibo"
	}
	return "snap-kakeibo-" + stage
}
