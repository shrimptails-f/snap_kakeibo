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

// NewListAnalysisRequestsContainer は共通コンテナへ list-analysis-requests 固有の依存性と access token の検証を追加する。
func NewListAnalysisRequestsContainer(cfg settings.ListAnalysisRequestsConfig, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create list-analysis-requests container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "list-analysis-requests", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "list-analysis-requests",
		func() settings.ListAnalysisRequestsConfig { return cfg },
		func(client *libdynamodb.Client, cfg settings.ListAnalysisRequestsConfig) application.AnalysisRequestLister {
			return infrastructure.DynamoDBAnalysisRequestRepository{Table: client.Table(cfg.AnalysisRequestsTable)}
		},
		application.NewListAnalysisRequestsUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveListAnalysisRequestsUsecase はコンテナから HTTP 層が依存する解析依頼一覧ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveListAnalysisRequestsUsecase(container *dig.Container) (application.ListAnalysisRequestsUsecaseInterface, error) {
	var usecase application.ListAnalysisRequestsUsecaseInterface
	if err := container.Invoke(func(resolved application.ListAnalysisRequestsUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve list-analysis-requests usecase: %w", err)
	}
	return usecase, nil
}
