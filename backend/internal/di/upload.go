package di

import (
	"fmt"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/library/ulid"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/infrastructure"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewUploadContainer は共通コンテナへ upload 固有の依存性と access token の検証を追加する。
func NewUploadContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create upload container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "upload", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "upload",
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.AnalysisRequestRepository {
			return infrastructure.DynamoDBAnalysisRequestRepository{Table: client.Table(cfg.AnalysisRequestsTable)}
		},
		func(client *libs3.Client, cfg settings.Config) application.UploadURLPresigner {
			return infrastructure.S3UploadPresigner{Bucket: client.Bucket(cfg.ReceiptBucket)}
		},
		func(clock timewrapper.Interface) application.IDGenerator { return ulid.New(clock) },
		application.NewCreateUploadUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveUploadUsecase はコンテナから HTTP 層が依存するアップロード開始ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveUploadUsecase(container *dig.Container) (application.CreateUploadUsecaseInterface, error) {
	var usecase application.CreateUploadUsecaseInterface
	if err := container.Invoke(func(resolved application.CreateUploadUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve upload usecase: %w", err)
	}
	return usecase, nil
}
