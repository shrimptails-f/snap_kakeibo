package main

import (
	"context"
	"errors"
	"time"

	authapp "snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/library/apigateway"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// item はレスポンスの 1 件。JSON キーは画面(front/src/App.tsx の UploadItem)が読む名前で、旧実装から変えない。
type item struct {
	UploadID     string `json:"upload_id"`
	BillingID    string `json:"billing_id,omitempty"`
	Status       string `json:"status"`
	FileName     string `json:"file_name"`
	ContentType  string `json:"content_type"`
	YearMonth    string `json:"year_month"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type response struct {
	Items []item `json:"items"`
}

var (
	check authapp.CheckUsecaseInterface
	list  application.ListUploadsUsecaseInterface
	log   logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.LoadListUploads(osw)
	if err != nil {
		panic(err)
	}
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewListUploadsContainer(cfg, awsCfg, osw, configuredLogger)
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
	list, err = di.ResolveListUploadsUsecase(container)
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

	month := req.PathParameters["month"]
	if month == "" {
		return apigateway.Error(400, "month path parameter is required")
	}
	out, err := list.List(ctx, application.ListUploadsInput{UserID: user.UserID, YearMonth: month})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return apigateway.Error(400, "month must be YYYY-MM")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	// 0 件でも null ではなく [] を返す(画面は items をそのまま map する)
	items := make([]item, 0, len(out.Uploads))
	for _, h := range out.Uploads {
		items = append(items, item{
			UploadID:     h.UploadID,
			BillingID:    h.BillingID,
			Status:       string(h.Status),
			FileName:     h.FileName,
			ContentType:  h.ContentType,
			YearMonth:    h.YearMonth,
			ErrorCode:    h.ErrorCode,
			ErrorMessage: h.ErrorMessage,
			CreatedAt:    h.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:    h.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return apigateway.JSON(200, response{Items: items})
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
