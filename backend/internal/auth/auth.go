package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
	CookieName      = "refresh_token"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
)

type Service struct {
	DDB *dynamodb.Client
	SSM *ssm.Client
	Cfg app.Config

	secretOnce sync.Once
	secret     string
	secretErr  error
}

type User struct {
	PK           string `dynamodbav:"PK"`
	Type         string `dynamodbav:"type"`
	UserID       string `dynamodbav:"user_id"`
	Email        string `dynamodbav:"email"`
	PasswordHash string `dynamodbav:"password_hash"`
	CreatedAt    string `dynamodbav:"created_at"`
	UpdatedAt    string `dynamodbav:"updated_at"`
}

type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

type Tokens struct {
	AccessToken           string
	ExpiresIn             int64
	RefreshToken          string
	RefreshTokenExpiresIn int64
}

func UserPKByEmail(email string) string {
	return "EMAIL#" + strings.ToLower(strings.TrimSpace(email))
}

func RefreshPK(tokenDigest string) string { return "REFRESH#" + tokenDigest }

func (s *Service) Login(ctx context.Context, email, password string) (Tokens, User, error) {
	user, err := s.getUserByEmail(ctx, email)
	if err != nil {
		return Tokens{}, User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Tokens{}, User{}, ErrInvalidCredentials
	}
	tokens, err := s.issueTokens(ctx, user)
	return tokens, user, err
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (Tokens, error) {
	digest := digestRefreshToken(rawRefreshToken)
	out, err := s.DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.Cfg.UsersTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digest)},
		},
	})
	if err != nil {
		return Tokens{}, err
	}
	if len(out.Item) == 0 {
		return Tokens{}, ErrUnauthorized
	}
	var rt refreshToken
	if err := attributevalue.UnmarshalMap(out.Item, &rt); err != nil {
		return Tokens{}, err
	}
	if rt.RevokedAt != "" || rt.ExpiresAt.Before(time.Now().UTC()) {
		return Tokens{}, ErrUnauthorized
	}
	user, err := s.getUserByEmail(ctx, rt.Email)
	if err != nil {
		return Tokens{}, err
	}
	_ = s.revokeRefreshToken(ctx, rawRefreshToken)
	return s.issueTokens(ctx, user)
}

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

func (s *Service) getUserByEmail(ctx context.Context, email string) (User, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return User{}, ErrInvalidCredentials
	}
	out, err := s.DDB.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.Cfg.UsersTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: UserPKByEmail(normalized)},
		},
	})
	if err != nil {
		return User{}, err
	}
	if len(out.Item) == 0 {
		return User{}, ErrInvalidCredentials
	}
	var user User
	if err := attributevalue.UnmarshalMap(out.Item, &user); err != nil {
		return User{}, err
	}
	if user.UserID == "" || user.PasswordHash == "" {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) issueTokens(ctx context.Context, user User) (Tokens, error) {
	now := time.Now().UTC()
	secret, err := s.jwtSecret(ctx)
	if err != nil {
		return Tokens{}, err
	}
	claims := Claims{
		UserID: user.UserID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer(),
			Subject:   user.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := generateRefreshToken()
	if err != nil {
		return Tokens{}, err
	}
	item, err := attributevalue.MarshalMap(refreshToken{
		PK:          RefreshPK(digestRefreshToken(refresh)),
		Type:        "REFRESH_TOKEN",
		UserID:      user.UserID,
		Email:       user.Email,
		ExpiresAt:   now.Add(RefreshTokenTTL),
		CreatedAt:   now,
		LastUsedAt:  now,
		RefreshHint: refresh[len(refresh)-8:],
	})
	if err != nil {
		return Tokens{}, err
	}
	_, err = s.DDB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.Cfg.UsersTable),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{
		AccessToken:           access,
		ExpiresIn:             int64(AccessTokenTTL / time.Second),
		RefreshToken:          refresh,
		RefreshTokenExpiresIn: int64(RefreshTokenTTL / time.Second),
	}, nil
}

func (s *Service) revokeRefreshToken(ctx context.Context, raw string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DDB.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.Cfg.UsersTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digestRefreshToken(raw))},
		},
		UpdateExpression: aws.String("SET revoked_at = :now"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":now": &ddbtypes.AttributeValueMemberS{Value: now},
		},
	})
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

func generateRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
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

type refreshToken struct {
	PK          string    `dynamodbav:"PK"`
	Type        string    `dynamodbav:"type"`
	UserID      string    `dynamodbav:"user_id"`
	Email       string    `dynamodbav:"email"`
	ExpiresAt   time.Time `dynamodbav:"expires_at"`
	CreatedAt   time.Time `dynamodbav:"created_at"`
	LastUsedAt  time.Time `dynamodbav:"last_used_at"`
	RefreshHint string    `dynamodbav:"refresh_hint"`
	RevokedAt   string    `dynamodbav:"revoked_at"`
}
