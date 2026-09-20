package di

import (
	"context"
	"fmt"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/infrastructure"
	"snap_kakeibo/backend/internal/analysis/library/image"
	"snap_kakeibo/backend/internal/analysis/library/settings"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/openai"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/library/ulid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewAnalyzeReceiptContainer は共通コンテナへ analyze-receipt 固有の依存性を追加する。
func NewAnalyzeReceiptContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create analyze-receipt container: %w", err)
	}
	if err := Provide(container, "analyze-receipt",
		func() settings.Config { return cfg },
		func(client *libs3.Client, cfg settings.Config) infrastructure.S3ReceiptStorage {
			return infrastructure.S3ReceiptStorage{Client: client, Results: client.Bucket(cfg.ReceiptBucket)}
		},
		func(storage infrastructure.S3ReceiptStorage) application.ReceiptImageReader { return storage },
		func(storage infrastructure.S3ReceiptStorage) application.RawResultStore { return storage },
		func(cfg settings.Config) application.ImageResizer { return image.Resizer{MaxEdge: cfg.ImageMaxEdge} },
		newOpenAIClient,
		func(client infrastructure.ResponsesClient, cfg settings.Config) application.ReceiptAnalyzer {
			return infrastructure.OpenAIReceiptAnalyzer{Client: client, Model: cfg.OpenAIModel, ReasoningEffort: cfg.OpenAIReasoningEffort}
		},
		func(client *libdynamodb.Client, cfg settings.Config) application.UploadHistoryRepository {
			return infrastructure.DynamoDBUploadHistoryRepository{Table: client.Table(cfg.UploadHistoriesTable)}
		},
		func(client *libdynamodb.Client, cfg settings.Config) application.BillingRegistrar {
			return infrastructure.DynamoDBBillingRegistrar{
				Client:           client,
				UploadHistories:  client.Table(cfg.UploadHistoriesTable),
				Billings:         client.Table(cfg.BillingsTable),
				BillingDetails:   client.Table(cfg.BillingDetailsTable),
				MonthlySummaries: client.Table(cfg.MonthlySummariesTable),
			}
		},
		func(clock timewrapper.Interface) application.IDGenerator { return ulid.New(clock) },
		application.NewAnalyzeReceiptUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// newOpenAIClient は SSM から API key を起動時に 1 回だけ取得して OpenAI client を作る。
// 起動時なので request の context はなく、取得に失敗すれば Lambda の初期化として失敗させる。
func newOpenAIClient(cfg settings.Config, ssmClient *libssm.Client, log logger.Interface, clock timewrapper.Interface) (infrastructure.ResponsesClient, error) {
	apiKey, err := ssmClient.Parameter(cfg.OpenAIAPIKeyParameter).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAI API key: %w", err)
	}
	client, err := openai.New(openai.Options{APIKey: apiKey, Logger: log, Clock: clock})
	if err != nil {
		return nil, fmt.Errorf("create OpenAI client: %w", err)
	}
	return client, nil
}

// ResolveAnalyzeReceiptUsecase はコンテナから Lambda 層が依存するユースケース契約を取り出す。
func ResolveAnalyzeReceiptUsecase(container *dig.Container) (application.AnalyzeReceiptUsecaseInterface, error) {
	var usecase application.AnalyzeReceiptUsecaseInterface
	if err := container.Invoke(func(resolved application.AnalyzeReceiptUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve analyze-receipt usecase: %w", err)
	}
	return usecase, nil
}
