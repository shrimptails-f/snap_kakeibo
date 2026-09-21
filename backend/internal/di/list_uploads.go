package di

import (
	"fmt"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/infrastructure"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewListUploadsContainer は共通コンテナへ list-uploads 固有の依存性と access token の検証を追加する。
func NewListUploadsContainer(cfg settings.ListUploadsConfig, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create list-uploads container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "list-uploads", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "list-uploads",
		func() settings.ListUploadsConfig { return cfg },
		func(client *libdynamodb.Client, cfg settings.ListUploadsConfig) application.UploadHistoryLister {
			return infrastructure.DynamoDBUploadHistoryRepository{Table: client.Table(cfg.UploadHistoriesTable)}
		},
		application.NewListUploadsUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveListUploadsUsecase はコンテナから HTTP 層が依存する一覧ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveListUploadsUsecase(container *dig.Container) (application.ListUploadsUsecaseInterface, error) {
	var usecase application.ListUploadsUsecaseInterface
	if err := container.Invoke(func(resolved application.ListUploadsUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve list-uploads usecase: %w", err)
	}
	return usecase, nil
}
