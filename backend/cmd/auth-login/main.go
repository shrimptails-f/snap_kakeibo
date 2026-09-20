package main

import (
	"context"
	"encoding/json"
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

type request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

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
	var in request
	if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
		return app.Error(400, "invalid JSON body")
	}
	tokens, user, err := svc.Login(ctx, in.Email, in.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return app.Error(401, "invalid email or password")
		}
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	res, err := app.JSON(200, map[string]any{
		"access_token": tokens.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   tokens.ExpiresIn,
		"user": map[string]string{
			"user_id": user.UserID,
			"email":   user.Email,
		},
	})
	res.Cookies = []string{auth.Cookie(tokens.RefreshToken, int(tokens.RefreshTokenExpiresIn))}
	return res, err
}

func main() { lambda.Start(handler) }
