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
	"snap_kakeibo/backend/internal/upload/domain"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// item はレスポンスの 1 件。JSON キーは画面(front/src/App.tsx の AnalysisRequestItem)が読む名前。
type item struct {
	AnalysisRequestID string  `json:"analysis_request_id"`
	ExpenseID         string  `json:"expense_id,omitempty"`
	Status            string  `json:"status"`
	Attempt           int     `json:"attempt"`
	FileName          string  `json:"file_name"`
	ContentType       string  `json:"content_type"`
	YearMonth         string  `json:"year_month"`
	UploadExpiresAt   string  `json:"upload_expires_at"`
	ErrorCode         string  `json:"error_code,omitempty"`
	ErrorMessage      string  `json:"error_message,omitempty"`
	FailedAt          string  `json:"failed_at,omitempty"`
	StoreName         *string `json:"store_name,omitempty"`
	RecordedAmount    *int64  `json:"recorded_amount,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type response struct {
	Items      []item `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

var (
	check authapp.CheckUsecaseInterface
	list  application.ListAnalysisRequestsUsecaseInterface
	log   logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.LoadListAnalysisRequests(osw)
	if err != nil {
		panic(err)
	}
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewListAnalysisRequestsContainer(cfg, awsCfg, osw, configuredLogger)
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
	list, err = di.ResolveListAnalysisRequestsUsecase(container)
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
	out, err := list.List(ctx, application.ListAnalysisRequestsInput{
		UserID: user.UserID, YearMonth: month,
		Filter: req.QueryStringParameters["filter"], Cursor: req.QueryStringParameters["cursor"],
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) || errors.Is(err, application.ErrInvalidCursor) {
			return apigateway.Error(400, "month, filter, or cursor is invalid")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	// 0 件でも null ではなく [] を返す(画面は items をそのまま map する)
	items := make([]item, 0, len(out.Items))
	for _, listItem := range out.Items {
		items = append(items, toItem(listItem))
	}
	return apigateway.JSON(200, response{Items: items, NextCursor: out.NextCursor})
}

// toItem は解析依頼の集約を HTTP レスポンスの 1 件へ変換する。失敗理由は FAILED のときだけ付く。
func toItem(listItem application.AnalysisRequestListItem) item {
	r := listItem.Request
	out := item{
		AnalysisRequestID: r.ID().String(),
		ExpenseID:         r.ExpenseID().String(),
		Status:            string(r.Status()),
		Attempt:           r.CurrentAttempt().Int(),
		FileName:          r.Image().FileName(),
		ContentType:       r.Image().ContentType(),
		YearMonth:         domain.YearMonth(r.CreatedAt()),
		UploadExpiresAt:   r.UploadExpiresAt().UTC().Format(time.RFC3339),
		CreatedAt:         r.CreatedAt().UTC().Format(time.RFC3339),
		UpdatedAt:         r.UpdatedAt().UTC().Format(time.RFC3339),
	}
	if reason, ok := r.FailureReason(); ok {
		out.ErrorCode, out.ErrorMessage = reason.Code(), reason.SafeMessage()
	}
	if failedAt, ok := r.FailedAt(); ok {
		out.FailedAt = failedAt.UTC().Format(time.RFC3339)
	}
	if listItem.ExpenseSummary != nil {
		out.StoreName = &listItem.ExpenseSummary.StoreName
		out.RecordedAmount = &listItem.ExpenseSummary.RecordedAmount
	}
	return out
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
