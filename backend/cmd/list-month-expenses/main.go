package main

import (
	"context"
	"errors"

	authapp "snap_kakeibo/backend/internal/auth/application"
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

type item struct {
	DetailID     string `json:"detail_id"`
	ExpenseID    string `json:"expense_id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Amount       int64  `json:"amount"`
	Quantity     int64  `json:"quantity"`
	Source       string `json:"source"`
	IsEdited     bool   `json:"is_edited"`
	StoreName    string `json:"store_name"`
	PurchaseDate string `json:"purchase_date"`
}
type response struct {
	YearMonth string `json:"year_month"`
	Items     []item `json:"items"`
}

var check authapp.CheckUsecaseInterface
var list application.ListMonthExpensesUsecaseInterface
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
	container, err := di.NewListMonthExpensesContainer(cfg, awsCfg, osw, configuredLogger)
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
	list, err = di.ResolveListMonthExpensesUsecase(container)
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
	out, err := list.List(ctx, application.ListMonthExpensesInput{UserID: user.UserID, YearMonth: req.PathParameters["month"]})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return apigateway.Error(400, "month must be YYYY-MM")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	items := make([]item, 0, len(out.Items))
	for _, value := range out.Items {
		items = append(items, item{DetailID: value.DetailID.String(), ExpenseID: value.ExpenseID.String(), Name: value.Name, Category: value.Category.String(), Amount: value.Amount, Quantity: value.Quantity, Source: value.Source.String(), IsEdited: value.IsEdited, StoreName: value.StoreName, PurchaseDate: value.PurchaseDate})
	}
	return apigateway.JSON(200, response{YearMonth: out.YearMonth, Items: items})
}
func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
