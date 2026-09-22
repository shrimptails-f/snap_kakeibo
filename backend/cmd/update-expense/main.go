package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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

type detailRequest struct {
	DetailID string `json:"detail_id"`
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Quantity int64  `json:"quantity"`
	Category string `json:"category"`
}
type request struct {
	StoreName        string          `json:"store_name"`
	PurchaseDate     string          `json:"purchase_date"`
	AdjustmentAmount int64           `json:"adjustment_amount"`
	Details          []detailRequest `json:"details"`
}
type expenseResponse struct {
	ExpenseID        string `json:"expense_id"`
	ReadAmount       int64  `json:"read_amount"`
	AdjustmentAmount int64  `json:"adjustment_amount"`
	RecordedAmount   int64  `json:"recorded_amount"`
	UpdatedAt        string `json:"updated_at"`
}
type response struct {
	Expense expenseResponse `json:"expense"`
}

var (
	check  authapp.CheckUsecaseInterface
	update application.UpdateExpenseUsecaseInterface
	log    logger.Interface
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
	container, err := di.NewLedgerWriteContainer(cfg, awsCfg, osw, configuredLogger, "update-expense")
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
	update, err = di.ResolveUpdateExpenseUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	user, err := check.Check(ctx, authapp.CheckInput{Authorization: authorization(req.Headers)})
	if err != nil {
		if errors.Is(err, authapp.ErrUnauthorized) {
			return apigateway.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	expenseID := req.PathParameters["expenseId"]
	if expenseID == "" {
		return apigateway.Error(400, "expenseId path parameter is required")
	}
	var body request
	decoder := json.NewDecoder(strings.NewReader(req.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return apigateway.Error(400, "invalid expense request")
	}
	details := make([]application.UpdateExpenseDetailInput, 0, len(body.Details))
	for _, d := range body.Details {
		details = append(details, application.UpdateExpenseDetailInput{DetailID: d.DetailID, Name: d.Name, Amount: d.Amount, Quantity: d.Quantity, Category: d.Category})
	}
	out, err := update.Update(ctx, application.UpdateExpenseInput{UserID: user.UserID, ExpenseID: expenseID, StoreName: body.StoreName, PurchaseDate: body.PurchaseDate, AdjustmentAmount: body.AdjustmentAmount, Details: details})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return apigateway.Error(400, "invalid expense request")
		case errors.Is(err, application.ErrExpenseNotFound):
			return apigateway.Error(404, "expense not found")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	e := out.Expense
	return apigateway.JSON(200, response{Expense: expenseResponse{ExpenseID: e.ID().String(), ReadAmount: e.ReadAmount().Yen(), AdjustmentAmount: e.AdjustmentAmount().Yen(), RecordedAmount: e.RecordedAmount().Yen(), UpdatedAt: out.UpdatedAt.UTC().Format(time.RFC3339)}})
}

func authorization(headers map[string]string) string {
	if value := headers["authorization"]; value != "" {
		return value
	}
	return headers["Authorization"]
}
func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
