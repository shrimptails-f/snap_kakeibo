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
	tokens, err := svc.Refresh(ctx, auth.ReadRefreshCookie(req))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, auth.ErrInvalidCredentials) {
			return app.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := app.JSON(200, map[string]any{
		"access_token": tokens.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   tokens.ExpiresIn,
	})
	res.Headers["cache-control"] = "no-store"
	res.Headers["pragma"] = "no-cache"
	res.Cookies = []string{auth.Cookie(tokens.RefreshToken, int(tokens.RefreshTokenExpiresIn))}
	return res, err
}

func main() { lambda.Start(handler) }
