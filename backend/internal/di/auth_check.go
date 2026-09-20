package di

import (
	"fmt"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/auth/library/token"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewAuthCheckContainer は共通コンテナへ auth-check 固有の依存性を追加する。
func NewAuthCheckContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create auth-check container: %w", err)
	}
	if err := Provide(container, "auth-check",
		func(client *libssm.Client) token.SecretProvider {
			if cfg.JWTSecret != "" {
				return token.StaticSecretProvider{Value: cfg.JWTSecret}
			}
			return &token.SSMSecretProvider{Parameter: client.Parameter(cfg.JWTSecretParameter)}
		},
		func(secrets token.SecretProvider, clock timewrapper.Interface) application.AccessTokenVerifier {
			return token.JWTVerifier{Secrets: secrets, Issuer: issuer(cfg.Stage), Clock: clock}
		},
		application.NewCheckUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveAuthCheckUsecase はコンテナから HTTP 層が依存するユースケース契約を取り出す。
func ResolveAuthCheckUsecase(container *dig.Container) (application.CheckUsecaseInterface, error) {
	var usecase application.CheckUsecaseInterface
	if err := container.Invoke(func(resolved application.CheckUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve auth-check usecase: %w", err)
	}
	return usecase, nil
}
