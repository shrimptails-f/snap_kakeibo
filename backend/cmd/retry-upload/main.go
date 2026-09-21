package main

import (
	"context"
	"errors"

	"snap_kakeibo/backend/internal/app"
	authapp "snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/di"
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

type response struct {
	UploadID string `json:"upload_id"`
	Status   string `json:"status"`
	Attempt  int    `json:"attempt"`
}

var (
	check authapp.CheckUsecaseInterface
	retry application.RetryUploadUsecaseInterface
	log   logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.LoadRetryUpload(osw)
	if err != nil {
		panic(err)
	}
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewRetryUploadContainer(cfg, awsCfg, osw, configuredLogger)
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
	retry, err = di.ResolveRetryUploadUsecase(container)
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

	uploadID := req.PathParameters["uploadId"]
	if uploadID == "" {
		return app.Error(400, "uploadId path parameter is required")
	}
	out, err := retry.Retry(ctx, application.RetryUploadInput{UserID: user.UserID, UploadID: uploadID})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return app.Error(400, "invalid retry request")
		case errors.Is(err, application.ErrUploadNotRetryable):
			return app.Error(409, "upload cannot be retried")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, response{UploadID: out.UploadID, Status: string(out.Status), Attempt: out.Attempt})
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
