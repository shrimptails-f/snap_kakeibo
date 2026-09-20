package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	loginAttemptLimit  = 20
	loginAttemptWindow = 5 * time.Minute
)

// DynamoDBLoginAttemptLimiter は固定時間窓の試行回数を DynamoDB で原子的に数える。
// subject はハッシュ化して保存し、メールアドレスや IP アドレスをテーブルに残さない。
type DynamoDBLoginAttemptLimiter struct {
	Table *libdynamodb.Table
	Clock timewrapper.Interface
}

var _ application.LoginAttemptLimiter = DynamoDBLoginAttemptLimiter{}

func (l DynamoDBLoginAttemptLimiter) Allow(ctx context.Context, subject string) (bool, error) {
	if l.Table == nil || l.Clock == nil {
		return false, fmt.Errorf("login attempt limiter is not configured")
	}
	now := l.Clock.Now().UTC()
	windowStart := now.Truncate(loginAttemptWindow)
	digest := sha256.Sum256([]byte(subject))
	pk := fmt.Sprintf("LOGIN_ATTEMPT#%s#%d", hex.EncodeToString(digest[:]), windowStart.Unix())
	expiresAt := windowStart.Add(loginAttemptWindow + time.Hour).Unix()

	_, err := l.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: pk},
		},
		UpdateExpression:    aws.String("SET expires_at = :expires_at ADD attempt_count :one"),
		ConditionExpression: aws.String("attribute_not_exists(attempt_count) OR attempt_count < :limit"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":one":        &ddbtypes.AttributeValueMemberN{Value: "1"},
			":limit":      &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", loginAttemptLimit)},
			":expires_at": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", expiresAt)},
		},
	})
	if err == nil {
		return true, nil
	}
	var conditional *ddbtypes.ConditionalCheckFailedException
	if errors.As(err, &conditional) {
		return false, application.ErrTooManyLoginAttempts
	}
	return false, err
}
