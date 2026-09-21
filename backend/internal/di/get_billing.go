package di

import (
	"fmt"

	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/infrastructure"
	"snap_kakeibo/backend/internal/billing/library/settings"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewGetBillingContainer は共通コンテナへ get-billing 固有の依存性と access token の検証を追加する。
func NewGetBillingContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create get-billing container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "get-billing", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "get-billing",
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.BillingFinder {
			return infrastructure.DynamoDBBillingRepository{Table: client.Table(cfg.BillingsTable)}
		},
		func(client *libdynamodb.Client, cfg settings.Config) application.BillingDetailLister {
			return infrastructure.DynamoDBBillingDetailRepository{Table: client.Table(cfg.BillingDetailsTable)}
		},
		application.NewGetBillingUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveGetBillingUsecase はコンテナから HTTP 層が依存する請求参照ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveGetBillingUsecase(container *dig.Container) (application.GetBillingUsecaseInterface, error) {
	var usecase application.GetBillingUsecaseInterface
	if err := container.Invoke(func(resolved application.GetBillingUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve get-billing usecase: %w", err)
	}
	return usecase, nil
}
