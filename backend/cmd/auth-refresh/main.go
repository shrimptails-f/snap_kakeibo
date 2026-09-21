package main

import (
	"context"
	"errors"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/library/cookie"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/library/apigateway"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var (
	refresh application.RefreshUsecaseInterface
	log     logger.Interface
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
	container, err := di.NewAuthRefreshContainer(cfg, awsCfg, osw, configuredLogger)
	if err != nil {
		panic(err)
	}
	log, err = di.ResolveLogger(container)
	if err != nil {
		panic(err)
	}
	refresh, err = di.ResolveAuthRefreshUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	out, err := refresh.Refresh(ctx, application.RefreshInput{RefreshToken: cookie.ReadRefresh(req.Cookies)})
	if err != nil {
		if errors.Is(err, application.ErrUnauthorized) {
			return apigateway.Error(401, "unauthorized")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := apigateway.JSON(200, map[string]any{
		"access_token": out.Tokens.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   out.Tokens.ExpiresIn,
	})
	res.Headers["cache-control"] = "no-store"
	res.Headers["pragma"] = "no-cache"
	res.Cookies = []string{cookie.Refresh(out.Tokens.RefreshToken, int(out.Tokens.RefreshTokenExpiresIn))}
	return res, err
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
