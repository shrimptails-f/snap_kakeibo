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

func NewGetMonthlySummariesContainer(cfg settings.Config, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container, err := NewContainer(awsCfg, osw, log)
	if err != nil {
		return nil, fmt.Errorf("create get-monthly-summaries container: %w", err)
	}
	if err := provideAccessTokenVerification(container, "get-monthly-summaries", cfg.JWTSecretParameter, cfg.Stage); err != nil {
		return nil, err
	}
	if err := Provide(container, "get-monthly-summaries",
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.MonthlySummaryLister {
			return infrastructure.DynamoDBMonthlySummaryRepository{Table: client.Table(cfg.MonthlySummariesTable)}
		},
		application.NewGetMonthlySummariesUsecase,
	); err != nil {
		return nil, err
	}
	return container, nil
}

func ResolveGetMonthlySummariesUsecase(container *dig.Container) (application.GetMonthlySummariesUsecaseInterface, error) {
	var usecase application.GetMonthlySummariesUsecaseInterface
	if err := container.Invoke(func(resolved application.GetMonthlySummariesUsecaseInterface) { usecase = resolved }); err != nil {
		return nil, fmt.Errorf("resolve get-monthly-summaries usecase: %w", err)
	}
	return usecase, nil
}
