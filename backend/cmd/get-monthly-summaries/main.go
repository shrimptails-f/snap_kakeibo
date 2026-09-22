package main

import (
	"context"
	"errors"

	authapp "snap_kakeibo/backend/internal/auth/application"
	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/ledger/application"
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

type summary struct {
	YearMonth           string           `json:"year_month"`
	TotalRecordedAmount int64            `json:"total_recorded_amount"`
	ExpenseCount        int64            `json:"expense_count"`
	DetailCount         int64            `json:"detail_count"`
	CategoryTotals      map[string]int64 `json:"category_totals"`
	UpdatedAt           string           `json:"updated_at"`
}
type response struct {
	MonthlySummaries []summary `json:"monthly_summaries"`
}

var check authapp.CheckUsecaseInterface
var get application.GetMonthlySummariesUsecaseInterface
var log logger.Interface

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
	container, err := di.NewGetMonthlySummariesContainer(cfg, awsCfg, osw, configuredLogger)
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
	get, err = di.ResolveGetMonthlySummariesUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	authorization := req.Headers["authorization"]
	if authorization == "" {
		authorization = req.Headers["Authorization"]
	}
	user, err := check.Check(ctx, authapp.CheckInput{Authorization: authorization})
	if err != nil {
		if errors.Is(err, authapp.ErrUnauthorized) {
			return apigateway.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	out, err := get.Get(ctx, application.GetMonthlySummariesInput{UserID: user.UserID})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	summaries := make([]summary, 0, len(out.Summaries))
	for _, value := range out.Summaries {
		totals := make(map[string]int64, len(common.Categories()))
		for _, category := range common.Categories() {
			totals[category.String()] = value.Summary.CategoryTotals[category]
		}
		summaries = append(summaries, summary{YearMonth: value.Summary.YearMonth.String(), TotalRecordedAmount: value.Summary.TotalRecordedAmount, ExpenseCount: value.Summary.ExpenseCount, DetailCount: value.Summary.DetailCount, CategoryTotals: totals, UpdatedAt: value.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")})
	}
	return apigateway.JSON(200, response{MonthlySummaries: summaries})
}
func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
