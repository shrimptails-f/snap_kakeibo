package di

import (
	"fmt"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/auth/library/token"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewAuthLogoutContainer は共通コンテナへ auth-logout 固有の依存性を追加する。
func NewAuthLogoutContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create auth-logout container: %w", err)
	}
	if err := Provide(container, "auth-logout",
		func() token.RefreshTokenGenerator { return token.RefreshTokenGenerator{} },
		func(generator token.RefreshTokenGenerator) application.RefreshTokenDigester { return generator },
		func(client *libdynamodb.Client) application.RefreshTokenRevoker {
			return infrastructure.DynamoDBRefreshTokenRepository{Table: client.Table(cfg.RefreshTokensTable)}
		},
		application.NewLogoutUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveAuthLogoutUsecase はコンテナから HTTP 層が依存するユースケース契約を取り出す。
func ResolveAuthLogoutUsecase(container *dig.Container) (application.LogoutUsecaseInterface, error) {
	var usecase application.LogoutUsecaseInterface
	if err := container.Invoke(func(resolved application.LogoutUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve auth-logout usecase: %w", err)
	}
	return usecase, nil
}
