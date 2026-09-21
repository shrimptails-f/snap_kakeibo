package main

import (
	"context"
	"errors"

	"snap_kakeibo/backend/internal/app"
	authapp "snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/library/settings"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// JSON キーは画面(front/src/App.tsx の Billing / Detail)が読む名前で、旧実装から変えない。
type billingResponse struct {
	BillingID      string `json:"billing_id"`
	UploadID       string `json:"upload_id"`
	StoreName      string `json:"store_name"`
	PurchasedAt    string `json:"purchased_at"`
	YearMonth      string `json:"year_month"`
	OriginalAmount int64  `json:"original_amount"`
	DiscountAmount int64  `json:"discount_amount"`
	FinalAmount    int64  `json:"final_amount"`
}

type detailResponse struct {
	DetailID       string `json:"detail_id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	CategorySource string `json:"category_source"`
	Amount         int64  `json:"amount"`
	Quantity       int64  `json:"quantity"`
}

type response struct {
	Billing billingResponse  `json:"billing"`
	Details []detailResponse `json:"details"`
}

var (
	check authapp.CheckUsecaseInterface
	get   application.GetBillingUsecaseInterface
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
	container, err := di.NewGetBillingContainer(cfg, awsCfg, osw, configuredLogger)
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
	get, err = di.ResolveGetBillingUsecase(container)
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
			return app.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}

	billingID := req.PathParameters["billingId"]
	if billingID == "" {
		return app.Error(400, "billingId path parameter is required")
	}
	out, err := get.Get(ctx, application.GetBillingInput{UserID: user.UserID, BillingID: billingID})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return app.Error(400, "invalid billing request")
		case errors.Is(err, application.ErrBillingNotFound):
			return app.Error(404, "billing not found")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	// 0 件でも null ではなく [] を返す(画面は details をそのまま map する)
	details := make([]detailResponse, 0, len(out.Details))
	for _, d := range out.Details {
		details = append(details, detailResponse{DetailID: d.ID, Name: d.Name, Category: d.Category, CategorySource: d.CategorySource, Amount: d.Amount, Quantity: d.Quantity})
	}
	b := out.Billing
	return app.JSON(200, response{
		Billing: billingResponse{
			BillingID: b.ID, UploadID: b.UploadID, StoreName: b.StoreName, PurchasedAt: b.PurchasedAt, YearMonth: b.YearMonth,
			OriginalAmount: b.OriginalAmount, DiscountAmount: b.DiscountAmount, FinalAmount: b.FinalAmount,
		},
		Details: details,
	})
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
