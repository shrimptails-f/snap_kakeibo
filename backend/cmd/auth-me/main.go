package main

import (
	"context"
	"errors"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/auth"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

var svc *auth.Service

func init() {
	cfg := app.LoadConfig()
	awsCfg, err := awsconfig.Load(context.Background(), oswrapper.New())
	if err != nil {
		panic(err)
	}
	svc = &auth.Service{DDB: dynamodb.NewFromConfig(awsCfg), SSM: ssm.NewFromConfig(awsCfg), Cfg: cfg}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	claims, err := svc.VerifyRequest(ctx, req)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			return app.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, map[string]any{"user": map[string]string{"user_id": claims.UserID, "email": claims.Email}})
}

func main() { lambda.Start(handler) }
