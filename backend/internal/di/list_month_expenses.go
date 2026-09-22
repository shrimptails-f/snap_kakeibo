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

func NewListMonthExpensesContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create list-month-expenses container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "list-month-expenses", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "list-month-expenses",
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.MonthlyExpenseLister {
			return infrastructure.DynamoDBExpenseRepository{ExpenseDetails: client.Table(cfg.ExpenseDetailsTable)}
		},
		application.NewListMonthExpensesUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

func ResolveListMonthExpensesUsecase(container *dig.Container) (application.ListMonthExpensesUsecaseInterface, error) {
	var usecase application.ListMonthExpensesUsecaseInterface
	if err := container.Invoke(func(resolved application.ListMonthExpensesUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve list-month-expenses usecase: %w", err)
	}
	return usecase, nil
}
