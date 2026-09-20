// Package auth は移行前の認証サービス。
// auth-login / auth-refresh は application / infrastructure へ移行済みで、残りは access token の検証(VerifyRequest)と
// auth-logout の refresh token 失効(Logout)だけを提供する。これらも #29 で feature パッケージへ移す。
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/golang-jwt/jwt/v5"
)

const CookieName = "refresh_token"

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

// RefreshPK は refresh-tokens テーブルの永続化キー。infrastructure.RefreshPK と同じ形。
func RefreshPK(tokenDigest string) string { return "REFRESH#" + tokenDigest }

// Logout は refresh token を失効させる。Cookie がなければ何もしない。
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	if strings.TrimSpace(rawRefreshToken) == "" {
		return nil
	}
	return s.revokeRefreshToken(ctx, rawRefreshToken)
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

// revokeRefreshToken は refresh-tokens テーブルのアイテムに失効時刻を書く。
// 存在しない digest に空のアイテムを作らないよう attribute_exists を条件にし、不一致は失効済みとみなす。
func (s *Service) revokeRefreshToken(ctx context.Context, raw string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DDB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.Cfg.RefreshTokensTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digestRefreshToken(raw))},
		},
		UpdateExpression:    aws.String("SET revoked_at = :now"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":now": &ddbtypes.AttributeValueMemberS{Value: now},
		},
	})
	var conditional *ddbtypes.ConditionalCheckFailedException
	if errors.As(err, &conditional) {
		return nil
	}
	return err
}

func (s *Service) jwtSecret(ctx context.Context) (string, error) {
	s.secretOnce.Do(func() {
		if s.Cfg.JWTSecret != "" {
			s.secret = s.Cfg.JWTSecret
			return
		}
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

func digestRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func Cookie(refreshToken string, maxAge int) string {
	c := http.Cookie{
		Name:     CookieName,
		Value:    refreshToken,
		Path:     "/api/auth",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	return c.String()
}

func ReadRefreshCookie(req events.APIGatewayV2HTTPRequest) string {
	for _, raw := range req.Cookies {
		header := http.Header{"Cookie": []string{raw}}
		for _, cookie := range (&http.Request{Header: header}).Cookies() {
			if cookie.Name == CookieName {
				return cookie.Value
			}
		}
	}
	return ""
}
