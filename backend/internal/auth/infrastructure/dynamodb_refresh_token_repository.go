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

// refreshTokenRecord は refresh-tokens テーブルのアイテム。
// ExpiresAt は TTL 属性なので Unix 秒の数値で持つ。RevokedAt は失効時にだけ書き込むため、未失効のアイテムには属性が存在しない。
type refreshTokenRecord struct {
	PK          string     `dynamodbav:"PK"`
	GSI1PK      string     `dynamodbav:"GSI1PK"`
	GSI1SK      string     `dynamodbav:"GSI1SK"`
	Type        string     `dynamodbav:"type"`
	UserID      string     `dynamodbav:"user_id"`
	Email       string     `dynamodbav:"email"`
	ExpiresAt   int64      `dynamodbav:"expires_at"`
	CreatedAt   time.Time  `dynamodbav:"created_at"`
	LastUsedAt  time.Time  `dynamodbav:"last_used_at"`
	RefreshHint string     `dynamodbav:"refresh_hint"`
	RevokedAt   *time.Time `dynamodbav:"revoked_at,omitempty"`
}

// DynamoDBRefreshTokenRepository は refresh token を専用テーブル(refresh-tokens)に保存する。
// digest を PK にして refresh 時は強整合の GetItem で引き、ユーザー単位の一括失効は GSI で引く。
type DynamoDBRefreshTokenRepository struct {
	Table *libdynamodb.Table
}

var (
	_ application.RefreshTokenRepository  = DynamoDBRefreshTokenRepository{}
	_ application.RefreshTokenFinder      = DynamoDBRefreshTokenRepository{}
	_ application.RefreshTokenRevoker     = DynamoDBRefreshTokenRepository{}
	_ application.RefreshTokenBulkRevoker = DynamoDBRefreshTokenRepository{}
)

// Save は token digest をキーにした refresh token を追加する。
func (r DynamoDBRefreshTokenRepository) Save(ctx context.Context, token authdomain.RefreshToken) error {
	item, err := attributevalue.MarshalMap(refreshTokenRecord{
		PK:          RefreshPK(token.Digest),
		GSI1PK:      RefreshUserGSI1PK(token.UserID),
		GSI1SK:      RefreshCreatedGSI1SK(token.CreatedAt, token.Digest),
		Type:        "REFRESH_TOKEN",
		UserID:      token.UserID.String(),
		Email:       token.Email,
		ExpiresAt:   token.ExpiresAt.Unix(),
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
		Key:            refreshKey(digest),
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
		ExpiresAt:  time.Unix(record.ExpiresAt, 0).UTC(),
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
	return r.revokeByKey(ctx, RefreshPK(digest), revokedAt)
}

// RevokeAllByUser は利用者の未失効の refresh token をすべて失効させる。
// GSI は結果整合なので直前に発行されたトークンを取りこぼす可能性はあるが、全端末ログアウト用途では許容する。
func (r DynamoDBRefreshTokenRepository) RevokeAllByUser(ctx context.Context, userID common.UserID, revokedAt time.Time) error {
	var startKey map[string]ddbtypes.AttributeValue
	for {
		out, err := r.Table.Query(ctx, &awssdk.QueryInput{
			IndexName:              aws.String(libdynamodb.RefreshTokenUserIndex),
			KeyConditionExpression: aws.String("GSI1PK = :user"),
			FilterExpression:       aws.String("attribute_not_exists(revoked_at)"),
			ProjectionExpression:   aws.String("PK"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":user": &ddbtypes.AttributeValueMemberS{Value: RefreshUserGSI1PK(userID)},
			},
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return err
		}
		for _, item := range out.Items {
			pk, ok := item["PK"].(*ddbtypes.AttributeValueMemberS)
			if !ok {
				continue
			}
			// 一覧取得から失効までの間に TTL で消えたトークンは失効済みとみなす
			if err := r.revokeByKey(ctx, pk.Value, revokedAt); err != nil && !errors.Is(err, application.ErrRefreshTokenNotFound) {
				return err
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		startKey = out.LastEvaluatedKey
	}
}

func (r DynamoDBRefreshTokenRepository) revokeByKey(ctx context.Context, pk string, revokedAt time.Time) error {
	value, err := attributevalue.Marshal(revokedAt.UTC())
	if err != nil {
		return err
	}
	_, err = r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                       map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: pk}},
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

func refreshKey(digest string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: RefreshPK(digest)}}
}

// RefreshPK は refresh token の digest から永続化キーを作る。
func RefreshPK(digest string) string { return "REFRESH#" + digest }

// RefreshUserGSI1PK は利用者単位で refresh token を引くための GSI パーティションキーを作る。
func RefreshUserGSI1PK(userID common.UserID) string { return "USER#" + userID.String() }

// RefreshCreatedGSI1SK は発行日時順に並べるための GSI ソートキーを作る。同時刻の衝突を digest で避ける。
func RefreshCreatedGSI1SK(createdAt time.Time, digest string) string {
	return "REFRESH_CREATED_AT#" + createdAt.UTC().Format(time.RFC3339) + "#" + digest
}
