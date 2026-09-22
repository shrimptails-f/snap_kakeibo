package di

import (
	"fmt"

	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/infrastructure"
	"snap_kakeibo/backend/internal/ledger/library/settings"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

func NewLedgerWriteContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface, service string) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create %s container: %w", service, err)
	}
	if err := provideAccessTokenVerification(container, service, cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, service,
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.ExpenseRepository {
			return infrastructure.DynamoDBExpenseRepository{Client: client, Expenses: client.Table(cfg.ExpensesTable), ExpenseDetails: client.Table(cfg.ExpenseDetailsTable)}
		},
		func(client *libdynamodb.Client, cfg settings.Config) application.MonthlySummaryRepository {
			return infrastructure.DynamoDBMonthlySummaryRepository{Table: client.Table(cfg.MonthlySummariesTable)}
		},
		application.NewRebuildMonthlySummaryUsecase,
		application.NewUpdateExpenseUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

func ResolveRebuildMonthlySummaryUsecase(container *dig.Container) (application.RebuildMonthlySummaryUsecaseInterface, error) {
	var usecase application.RebuildMonthlySummaryUsecaseInterface
	if err := container.Invoke(func(resolved application.RebuildMonthlySummaryUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve rebuild monthly summary usecase: %w", err)
	}
	return usecase, nil
}

func ResolveUpdateExpenseUsecase(container *dig.Container) (application.UpdateExpenseUsecaseInterface, error) {
	var usecase application.UpdateExpenseUsecaseInterface
	if err := container.Invoke(func(resolved application.UpdateExpenseUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve update expense usecase: %w", err)
	}
	return usecase, nil
}
