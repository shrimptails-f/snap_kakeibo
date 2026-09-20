package main

import (
	"context"

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
	if err := svc.Logout(ctx, auth.ReadRefreshCookie(req)); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := app.JSON(204, map[string]string{})
	res.Cookies = []string{auth.Cookie("", -1)}
	return res, err
}

func main() { lambda.Start(handler) }
