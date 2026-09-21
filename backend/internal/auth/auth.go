// Package auth は移行前の access token 検証サービス。
// auth-login / auth-refresh / auth-logout / auth-check は application / infrastructure へ移行済み。
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthorized = errors.New("unauthorized")

type Service struct {
	DDB *dynamodb.Client
	SSM *ssm.Client
	Cfg app.Config

	secretOnce sync.Once
	secret     string
	secretErr  error
}

type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

func (s *Service) VerifyRequest(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Claims, error) {
	h := req.Headers["authorization"]
	if h == "" {
		h = req.Headers["Authorization"]
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return Claims{}, ErrUnauthorized
	}
	return s.VerifyAccessToken(ctx, strings.TrimSpace(strings.TrimPrefix(h, prefix)))
}

func (s *Service) VerifyAccessToken(ctx context.Context, raw string) (Claims, error) {
	secret, err := s.jwtSecret(ctx)
	if err != nil {
		return Claims{}, err
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return []byte(secret), nil
	}, jwt.WithIssuer(s.issuer()))
	if err != nil || !token.Valid || claims.UserID == "" {
		return Claims{}, ErrUnauthorized
	}
	return *claims, nil
}

func (s *Service) jwtSecret(ctx context.Context) (string, error) {
	s.secretOnce.Do(func() {
		if s.Cfg.JWTSecretParameter == "" {
			s.secretErr = fmt.Errorf("SSM_JWT_SECRET is required")
			return
		}
		out, err := s.SSM.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           aws.String(s.Cfg.JWTSecretParameter),
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			s.secretErr = err
			return
		}
		s.secret = aws.ToString(out.Parameter.Value)
		if s.secret == "" {
			s.secretErr = fmt.Errorf("JWT secret is empty")
		}
	})
	return s.secret, s.secretErr
}

func (s *Service) issuer() string {
	if s.Cfg.Stage == "" {
		return "snap-kakeibo"
	}
	return "snap-kakeibo-" + s.Cfg.Stage
}
