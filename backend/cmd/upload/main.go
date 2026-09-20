package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

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

type request struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
}

type response struct {
	UploadID  string `json:"upload_id"`
	S3Key     string `json:"s3_key"`
	PutURL    string `json:"put_url"`
	ExpiresAt string `json:"expires_at"`
}

var (
	check  authapp.CheckUsecaseInterface
	create application.CreateUploadUsecaseInterface
	log    logger.Interface
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
	container, err := di.NewUploadContainer(cfg, awsCfg, osw, configuredLogger)
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
	create, err = di.ResolveUploadUsecase(container)
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

	var in request
	if req.Body != "" {
		if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
			return app.Error(400, "invalid JSON body")
		}
	}
	out, err := create.Create(ctx, application.CreateUploadInput{UserID: user.UserID, FileName: in.FileName, ContentType: in.ContentType})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return app.Error(400, "invalid upload request")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, response{
		UploadID:  out.UploadID,
		S3Key:     out.S3Key,
		PutURL:    out.PutURL,
		ExpiresAt: out.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
