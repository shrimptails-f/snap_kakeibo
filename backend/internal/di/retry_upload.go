package di

import (
	"fmt"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libsqs "snap_kakeibo/backend/internal/library/sqs"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/infrastructure"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewRetryUploadContainer は共通コンテナへ retry-upload 固有の依存性と access token の検証を追加する。
func NewRetryUploadContainer(cfg settings.RetryUploadConfig, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create retry-upload container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "retry-upload", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "retry-upload",
		func() settings.RetryUploadConfig { return cfg },
		func(client *libdynamodb.Client, cfg settings.RetryUploadConfig) application.UploadRetryMarker {
			return infrastructure.DynamoDBUploadHistoryRepository{Table: client.Table(cfg.UploadHistoriesTable)}
		},
		func(client *libsqs.Client, cfg settings.RetryUploadConfig) application.AnalyzeJobEnqueuer {
			return infrastructure.SQSAnalyzeQueue{Queue: client.Queue(cfg.AnalyzeQueueURL)}
		},
		application.NewRetryUploadUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveRetryUploadUsecase はコンテナから HTTP 層が依存する再実行ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveRetryUploadUsecase(container *dig.Container) (application.RetryUploadUsecaseInterface, error) {
	var usecase application.RetryUploadUsecaseInterface
	if err := container.Invoke(func(resolved application.RetryUploadUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve retry-upload usecase: %w", err)
	}
	return usecase, nil
}
