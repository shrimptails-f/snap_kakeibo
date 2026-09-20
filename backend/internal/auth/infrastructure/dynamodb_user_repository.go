// Package infrastructure は認証業務の外部サービス実装を提供する。
package infrastructure

import (
	"context"
	"strings"

	"snap_kakeibo/backend/internal/auth/application"
	common "snap_kakeibo/backend/internal/common/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type userRecord struct {
	UserID       string `dynamodbav:"user_id"`
	Email        string `dynamodbav:"email"`
	PasswordHash string `dynamodbav:"password_hash"`
}

// DynamoDBUserRepository は UsersTable から利用者を取得する。
type DynamoDBUserRepository struct {
	Table *libdynamodb.Table
}

var _ application.UserRepository = DynamoDBUserRepository{}

// FindByEmail はメールアドレスの一意キーで利用者を検索する。
func (r DynamoDBUserRepository) FindByEmail(ctx context.Context, email string) (common.User, error) {
	out, err := r.Table.GetItem(ctx, &awssdk.GetItemInput{
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: UserPKByEmail(email)},
		},
	})
	if err != nil {
		return common.User{}, err
	}
	if len(out.Item) == 0 {
		return common.User{}, application.ErrUserNotFound
	}
	var record userRecord
	if err := attributevalue.UnmarshalMap(out.Item, &record); err != nil {
		return common.User{}, err
	}
	id, ok := common.NewUserID(record.UserID)
	if !ok || strings.TrimSpace(record.PasswordHash) == "" {
		return common.User{}, application.ErrUserNotFound
	}
	return common.User{ID: id, Email: record.Email, PasswordHash: record.PasswordHash}, nil
}

// UserPKByEmail は UsersTable のメールアドレス検索キーを返す。
func UserPKByEmail(email string) string {
	return "EMAIL#" + strings.ToLower(strings.TrimSpace(email))
}
