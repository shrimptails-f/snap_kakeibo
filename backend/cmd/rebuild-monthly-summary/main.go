package main

import (
	"context"
	"errors"
	"time"

	authapp "snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/ledger/library/settings"
	"snap_kakeibo/backend/internal/library/apigateway"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

type response struct {
	YearMonth            string           `json:"year_month"`
	TotalRecordedAmount  int64            `json:"total_recorded_amount"`
	ExpenseCount         int64            `json:"expense_count"`
	DetailCount          int64            `json:"detail_count"`
	ConfirmedDetailCount int64            `json:"confirmed_detail_count"`
	CategoryTotals       map[string]int64 `json:"category_totals"`
	UpdatedAt            string           `json:"updated_at"`
}

var (
	check   authapp.CheckUsecaseInterface
	rebuild application.RebuildMonthlySummaryUsecaseInterface
	log     logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.Load(osw)
	if err != nil {
		panic(err)
	}
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewLedgerWriteContainer(cfg, awsCfg, osw, configuredLogger, "rebuild-monthly-summary")
	if err != nil {
		panic(err)
	}
	log, err = di.ResolveLogger(container)
	if err != nil {
		panic(err)
	}
	check, err = di.ResolveAuthCheckUsecase(container)
	if err != nil {
		panic(err)
	}
	rebuild, err = di.ResolveRebuildMonthlySummaryUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	auth := req.Headers["authorization"]
	if auth == "" {
		auth = req.Headers["Authorization"]
	}
	user, err := check.Check(ctx, authapp.CheckInput{Authorization: auth})
	if err != nil {
		if errors.Is(err, authapp.ErrUnauthorized) {
			return apigateway.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	out, err := rebuild.Rebuild(ctx, application.RebuildMonthlySummaryInput{UserID: user.UserID, YearMonth: req.PathParameters["month"]})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return apigateway.Error(400, "invalid monthly summary request")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return apigateway.JSON(200, toResponse(out.Summary, out.UpdatedAt))
}

func toResponse(summary domain.MonthlySummary, updatedAt time.Time) response {
	totals := make(map[string]int64, len(summary.CategoryTotals))
	for category, amount := range summary.CategoryTotals {
		totals[category.String()] = amount
	}
	return response{YearMonth: summary.YearMonth.String(), TotalRecordedAmount: summary.TotalRecordedAmount, ExpenseCount: summary.ExpenseCount, DetailCount: summary.DetailCount, ConfirmedDetailCount: summary.ConfirmedDetailCount, CategoryTotals: totals, UpdatedAt: updatedAt.UTC().Format(time.RFC3339)}
}
func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
