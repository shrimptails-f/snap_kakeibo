package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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

type request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var (
	login   application.LoginUsecaseInterface
	limiter application.LoginAttemptLimiter
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
	limiter, err = di.ResolveLoginAttemptLimiter(container)
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var in request
	if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
		return apigateway.Error(400, "invalid JSON body")
	}
	for _, subject := range []string{
		"ip:" + req.RequestContext.HTTP.SourceIP,
		"email:" + strings.ToLower(strings.TrimSpace(in.Email)),
	} {
		allowed, err := limiter.Allow(ctx, subject)
		if err != nil {
			if errors.Is(err, application.ErrTooManyLoginAttempts) {
				return apigateway.Error(429, "too many login attempts")
			}
			return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
		}
		if !allowed {
			return apigateway.Error(429, "too many login attempts")
		}
	}
	out, err := login.Login(ctx, application.LoginInput{Email: in.Email, Password: in.Password})
	if err != nil {
		if errors.Is(err, application.ErrInvalidCredentials) {
			return apigateway.Error(401, "invalid email or password")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := apigateway.JSON(200, map[string]any{
		"access_token": out.Tokens.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   out.Tokens.ExpiresIn,
		"user": map[string]string{
			"user_id": out.User.ID.String(),
			"email":   out.User.Email,
		},
	})
	res.Headers["cache-control"] = "no-store"
	res.Headers["pragma"] = "no-cache"
	res.Cookies = []string{cookie.Refresh(out.Tokens.RefreshToken, int(out.Tokens.RefreshTokenExpiresIn))}
	return res, err
}

func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
