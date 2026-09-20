package main

import (
	"context"
	"encoding/json"
	"errors"

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

type request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var (
	login application.LoginUsecaseInterface
	log   logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.LoadLoginConfig(osw)
	if err != nil {
		panic(err)
	}
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewAuthLoginContainer(cfg, awsCfg, osw, configuredLogger)
	if err != nil {
		panic(err)
	}
	log, err = di.ResolveLogger(container)
	if err != nil {
		panic(err)
	}
	login, err = di.ResolveAuthLoginUsecase(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var in request
	if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
		return app.Error(400, "invalid JSON body")
	}
	out, err := login.Login(ctx, application.LoginInput{Email: in.Email, Password: in.Password})
	if err != nil {
		if errors.Is(err, application.ErrInvalidCredentials) {
			return app.Error(401, "invalid email or password")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := app.JSON(200, map[string]any{
		"access_token": out.Tokens.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   out.Tokens.ExpiresIn,
		"user": map[string]string{
			"user_id": out.User.ID.String(),
			"email":   out.User.Email,
		},
	})
	res.Cookies = []string{cookie.Refresh(out.Tokens.RefreshToken, int(out.Tokens.RefreshTokenExpiresIn))}
	return res, err
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
