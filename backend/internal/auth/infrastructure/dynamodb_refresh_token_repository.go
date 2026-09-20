package infrastructure

import (
	"context"
	"errors"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// refreshTokenRecord は UsersTable 上の refresh token アイテム。
// RevokedAt は失効時にだけ書き込むため、未失効のアイテムには属性が存在しない。
type refreshTokenRecord struct {
	PK          string     `dynamodbav:"PK"`
	Type        string     `dynamodbav:"type"`
	UserID      string     `dynamodbav:"user_id"`
	Email       string     `dynamodbav:"email"`
	ExpiresAt   time.Time  `dynamodbav:"expires_at"`
	CreatedAt   time.Time  `dynamodbav:"created_at"`
	LastUsedAt  time.Time  `dynamodbav:"last_used_at"`
	RefreshHint string     `dynamodbav:"refresh_hint"`
	RevokedAt   *time.Time `dynamodbav:"revoked_at,omitempty"`
}

// DynamoDBRefreshTokenRepository は refresh token を UsersTable に保存する。
type DynamoDBRefreshTokenRepository struct {
	Table *libdynamodb.Table
}

var (
	_ application.RefreshTokenRepository = DynamoDBRefreshTokenRepository{}
	_ application.RefreshTokenFinder     = DynamoDBRefreshTokenRepository{}
	_ application.RefreshTokenRevoker    = DynamoDBRefreshTokenRepository{}
)

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

// FindByDigest は digest をキーに保存済みの refresh token を返す。
// 失効済み・期限切れの判定は行わず、そのまま返す。
func (r DynamoDBRefreshTokenRepository) FindByDigest(ctx context.Context, digest string) (authdomain.RefreshToken, error) {
	out, err := r.Table.GetItem(ctx, &awssdk.GetItemInput{
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digest)},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return authdomain.RefreshToken{}, err
	}
	if len(out.Item) == 0 {
		return authdomain.RefreshToken{}, application.ErrRefreshTokenNotFound
	}
	var record refreshTokenRecord
	if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
		return authdomain.RefreshToken{}, err
	}
	userID, ok := common.NewUserID(record.UserID)
	if !ok {
		return authdomain.RefreshToken{}, application.ErrRefreshTokenNotFound
	}
	token := authdomain.RefreshToken{
		Digest:     digest,
		UserID:     userID,
		Email:      record.Email,
		ExpiresAt:  record.ExpiresAt,
		CreatedAt:  record.CreatedAt,
		LastUsedAt: record.LastUsedAt,
		Hint:       record.RefreshHint,
	}
	if record.RevokedAt != nil {
		token.RevokedAt = *record.RevokedAt
	}
	return token, nil
}

// Revoke は保存済みの refresh token に失効時刻を記録する。
// 存在しない digest に対しては新規アイテムを作らず ErrRefreshTokenNotFound を返す。
func (r DynamoDBRefreshTokenRepository) Revoke(ctx context.Context, digest string, revokedAt time.Time) error {
	value, err := attributevalue.Marshal(revokedAt.UTC())
	if err != nil {
		return err
	}
	_, err = r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digest)},
		},
		UpdateExpression:          aws.String("SET revoked_at = :revoked_at"),
		ConditionExpression:       aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":revoked_at": value},
	})
	var conditional *ddbtypes.ConditionalCheckFailedException
	if errors.As(err, &conditional) {
		return application.ErrRefreshTokenNotFound
	}
	return err
}

// RefreshPK は refresh token の digest から永続化キーを作る。
func RefreshPK(digest string) string { return "REFRESH#" + digest }
