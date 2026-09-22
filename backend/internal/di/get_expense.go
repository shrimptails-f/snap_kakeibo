package di

import (
	"fmt"

	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/infrastructure"
	"snap_kakeibo/backend/internal/ledger/library/settings"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libs3 "snap_kakeibo/backend/internal/library/s3"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewGetExpenseContainer は共通コンテナへ get-expense 固有の依存性と access token の検証を追加する。
func NewGetExpenseContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create get-expense container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "get-expense", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "get-expense",
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.ExpenseFinder {
			return infrastructure.DynamoDBExpenseRepository{Expenses: client.Table(cfg.ExpensesTable), ExpenseDetails: client.Table(cfg.ExpenseDetailsTable)}
		},
		func(client *libs3.Client, cfg settings.Config) application.ReceiptImageURLPresigner {
			return infrastructure.S3ReceiptImageURLPresigner{Bucket: client.Bucket(cfg.ReceiptBucket)}
		},
		application.NewGetExpenseUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

// ResolveGetExpenseUsecase はコンテナから HTTP 層が依存する支出取得ユースケースを取り出す。
// 認証状態確認ユースケースは ResolveAuthCheckUsecase で取り出す。
func ResolveGetExpenseUsecase(container *dig.Container) (application.GetExpenseUsecaseInterface, error) {
	var usecase application.GetExpenseUsecaseInterface
	if err := container.Invoke(func(resolved application.GetExpenseUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve get-expense usecase: %w", err)
	}
	return usecase, nil
}
