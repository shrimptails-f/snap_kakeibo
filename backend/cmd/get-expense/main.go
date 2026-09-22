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

// JSON キーは画面(front/src/App.tsx の Expense / Detail)が読む名前。ユビキタス言語の読取金額 / 調整額 / 計上額に対応する。
type expenseResponse struct {
	ExpenseID         string `json:"expense_id"`
	AnalysisRequestID string `json:"analysis_request_id"`
	StoreName         string `json:"store_name"`
	PurchaseDate      string `json:"purchase_date"`
	YearMonth         string `json:"year_month"`
	ReadAmount        int64  `json:"read_amount"`
	AdjustmentAmount  int64  `json:"adjustment_amount"`
	RecordedAmount    int64  `json:"recorded_amount"`
	IsEdited          bool   `json:"is_edited"`
	Source            string `json:"source"`
	UpdatedAt         string `json:"updated_at"`
	ImageURL          string `json:"image_url"`
}

type detailResponse struct {
	DetailID       string `json:"detail_id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	CategorySource string `json:"category_source"`
	Amount         int64  `json:"amount"`
	Quantity       int64  `json:"quantity"`
	Source         string `json:"source"`
	IsEdited       bool   `json:"is_edited"`
}

type response struct {
	Expense expenseResponse  `json:"expense"`
	Details []detailResponse `json:"details"`
}

var (
	check authapp.CheckUsecaseInterface
	get   application.GetExpenseUsecaseInterface
	log   logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.Load(osw)
	if err != nil {
		panic(err)
	}
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewGetExpenseContainer(cfg, awsCfg, osw, configuredLogger)
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
	get, err = di.ResolveGetExpenseUsecase(container)
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

	expenseID := req.PathParameters["expenseId"]
	if expenseID == "" {
		return apigateway.Error(400, "expenseId path parameter is required")
	}
	out, err := get.Get(ctx, application.GetExpenseInput{UserID: user.UserID, ExpenseID: expenseID})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return apigateway.Error(400, "invalid expense request")
		case errors.Is(err, application.ErrExpenseNotFound):
			return apigateway.Error(404, "expense not found")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return apigateway.JSON(200, toResponse(out.Expense, out.ImageURL))
}

// toResponse は支出集約を HTTP レスポンスへ変換する。明細は 0 件でも null ではなく [] にする(画面は details をそのまま map する)。
func toResponse(e domain.Expense, imageURL string) response {
	source := e.Details()
	details := make([]detailResponse, 0, len(source))
	for _, d := range source {
		details = append(details, detailResponse{DetailID: d.ID().String(), Name: d.Name(), Category: d.Category().String(), CategorySource: d.CategorySource().String(), Amount: d.Amount().Yen(), Quantity: d.Quantity().Int64(), Source: d.Source().String(), IsEdited: d.Edited()})
	}
	return response{
		Expense: expenseResponse{
			ExpenseID: e.ID().String(), AnalysisRequestID: e.SourceRequestID().String(), StoreName: e.StoreName(),
			PurchaseDate: e.PurchaseDate().String(), YearMonth: e.PurchaseDate().YearMonth().String(),
			ReadAmount: e.ReadAmount().Yen(), AdjustmentAmount: e.AdjustmentAmount().Yen(), RecordedAmount: e.RecordedAmount().Yen(), IsEdited: e.Edited(),
			Source: e.Source().String(), UpdatedAt: e.UpdatedAt().UTC().Format(time.RFC3339),
			ImageURL: imageURL,
		},
		Details: details,
	}
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
