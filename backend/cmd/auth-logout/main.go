package main

import (
	"context"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/library/cookie"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

var (
	logout application.LogoutUsecaseInterface
	log    logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.LoadLogout(osw)
	if err != nil {
		panic(err)
	}
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewAuthLogoutContainer(cfg, awsCfg, osw, configuredLogger)
	if err != nil {
		panic(err)
	}
	log, err = di.ResolveLogger(container)
	if err != nil {
		panic(err)
	}
	logout, err = di.ResolveAuthLogoutUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	if err := logout.Logout(ctx, application.LogoutInput{RefreshToken: cookie.ReadRefresh(req.Cookies)}); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := app.JSON(204, map[string]string{})
	res.Cookies = []string{cookie.Refresh("", -1)}
	return res, err
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
