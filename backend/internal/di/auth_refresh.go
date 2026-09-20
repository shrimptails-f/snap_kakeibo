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

// NewAuthRefreshContainer は共通コンテナへ auth-refresh 固有の依存性を追加する。
func NewAuthRefreshContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create auth-refresh container: %w", err)
	}
	if err := provideAuthTokenDependencies(container, "auth-refresh", cfg); err != nil {
		return nil, err
	}
	if err := Provide(container, "auth-refresh",
		// 保存時と同じ算出方法で digest を求めるため、generator を digester としても使う。
		func(generator token.RefreshTokenGenerator) application.RefreshTokenDigester { return generator },
		func(client *libdynamodb.Client, cfg settings.Config) (application.RefreshTokenFinder, application.RefreshTokenRevoker) {
			repository := infrastructure.DynamoDBRefreshTokenRepository{Table: client.Table(cfg.UsersTable)}
			return repository, repository
		},
		application.NewRefreshUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveAuthRefreshUsecase はコンテナから HTTP 層が依存するユースケース契約を取り出す。
func ResolveAuthRefreshUsecase(container *dig.Container) (application.RefreshUsecaseInterface, error) {
	var usecase application.RefreshUsecaseInterface
	if err := container.Invoke(func(resolved application.RefreshUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve auth-refresh usecase: %w", err)
	}
	return usecase, nil
}
