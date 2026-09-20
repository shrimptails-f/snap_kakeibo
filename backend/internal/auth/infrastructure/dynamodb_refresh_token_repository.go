package infrastructure

import (
	"context"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type refreshTokenRecord struct {
	PK          string    `dynamodbav:"PK"`
	Type        string    `dynamodbav:"type"`
	UserID      string    `dynamodbav:"user_id"`
	Email       string    `dynamodbav:"email"`
	ExpiresAt   time.Time `dynamodbav:"expires_at"`
	CreatedAt   time.Time `dynamodbav:"created_at"`
	LastUsedAt  time.Time `dynamodbav:"last_used_at"`
	RefreshHint string    `dynamodbav:"refresh_hint"`
}

// DynamoDBRefreshTokenRepository は refresh token を UsersTable に保存する。
type DynamoDBRefreshTokenRepository struct {
	Table *libdynamodb.Table
}

var _ application.RefreshTokenRepository = DynamoDBRefreshTokenRepository{}

// Save は token digest をキーにした refresh token を追加する。
func (r DynamoDBRefreshTokenRepository) Save(ctx context.Context, token authdomain.RefreshToken) error {
	item, err := attributevalue.MarshalMap(refreshTokenRecord{
		PK:          RefreshPK(token.Digest),
		Type:        "REFRESH_TOKEN",
		UserID:      token.UserID.String(),
		Email:       token.Email,
		ExpiresAt:   token.ExpiresAt.UTC(),
		CreatedAt:   token.CreatedAt.UTC(),
		LastUsedAt:  token.LastUsedAt.UTC(),
		RefreshHint: token.Hint,
	})
	if err != nil {
		return err
	}
	_, err = r.Table.PutItem(ctx, &awssdk.PutItemInput{
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	return err
}

// RefreshPK は refresh token の digest から永続化キーを作る。
func RefreshPK(digest string) string { return "REFRESH#" + digest }
