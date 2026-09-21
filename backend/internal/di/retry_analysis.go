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

// NewRetryAnalysisContainer は共通コンテナへ retry-analysis 固有の依存性と access token の検証を追加する。
func NewRetryAnalysisContainer(cfg settings.RetryAnalysisConfig, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create retry-analysis container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "retry-analysis", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "retry-analysis",
		func() settings.RetryAnalysisConfig { return cfg },
		func(client *libdynamodb.Client, cfg settings.RetryAnalysisConfig) application.RetryAnalysisMarker {
			return infrastructure.DynamoDBAnalysisRequestRepository{Table: client.Table(cfg.AnalysisRequestsTable)}
		},
		func(client *libsqs.Client, cfg settings.RetryAnalysisConfig) application.RetryAnalysisEnqueuer {
			return infrastructure.SQSAnalyzeQueue{Queue: client.Queue(cfg.AnalyzeQueueURL)}
		},
		application.NewRetryAnalysisUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveRetryAnalysisUsecase はコンテナから HTTP 層が依存する再解析ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveRetryAnalysisUsecase(container *dig.Container) (application.RetryAnalysisUsecaseInterface, error) {
	var usecase application.RetryAnalysisUsecaseInterface
	if err := container.Invoke(func(resolved application.RetryAnalysisUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve retry-analysis usecase: %w", err)
	}
	return usecase, nil
}
