package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBLoginAttemptLimiterAllow(t *testing.T) {
	t.Parallel()
	api := &loginAttemptAPI{}
	now := time.Date(2026, 9, 20, 12, 3, 4, 0, time.UTC)
	limiter := DynamoDBLoginAttemptLimiter{
		Table: libdynamodb.NewWithAPI(api, nil).Table("users-test"),
		Clock: timewrapper.NewFixed(now),
	}

	allowed, err := limiter.Allow(context.Background(), "email:member@example.com")
	if err != nil || !allowed {
		t.Fatalf("Allow() = %v, %v", allowed, err)
	}
	if got := aws.ToString(api.input.TableName); got != "users-test" {
		t.Errorf("table name = %q", got)
	}
	if got := api.input.Key["PK"].(*ddbtypes.AttributeValueMemberS).Value; !strings.HasPrefix(got, "LOGIN_ATTEMPT#") || strings.Contains(got, "member@example.com") {
		t.Errorf("PK = %q: it must be a hashed login-attempt key", got)
	}
	if got := aws.ToString(api.input.ConditionExpression); got != "attribute_not_exists(attempt_count) OR attempt_count < :limit" {
		t.Errorf("condition = %q", got)
	}
	if got := api.input.ExpressionAttributeValues[":expires_at"].(*ddbtypes.AttributeValueMemberN).Value; got != "1789909500" {
		t.Errorf("expires_at = %s, want 1789909500", got)
	}
}

func TestDynamoDBLoginAttemptLimiterRejectsConditionalFailure(t *testing.T) {
	t.Parallel()
	api := &loginAttemptAPI{err: &ddbtypes.ConditionalCheckFailedException{}}
	limiter := DynamoDBLoginAttemptLimiter{
		Table: libdynamodb.NewWithAPI(api, nil).Table("users-test"),
		Clock: timewrapper.NewFixed(time.Now()),
	}

	allowed, err := limiter.Allow(context.Background(), "ip:192.0.2.1")
	if allowed || !errors.Is(err, application.ErrTooManyLoginAttempts) {
		t.Fatalf("Allow() = %v, %v", allowed, err)
	}
}

type loginAttemptAPI struct {
	input *awssdk.UpdateItemInput
	err   error
}

func (a *loginAttemptAPI) GetItem(context.Context, *awssdk.GetItemInput, ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	return &awssdk.GetItemOutput{}, nil
}

func (a *loginAttemptAPI) PutItem(context.Context, *awssdk.PutItemInput, ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	return &awssdk.PutItemOutput{}, nil
}

func (a *loginAttemptAPI) UpdateItem(_ context.Context, input *awssdk.UpdateItemInput, _ ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error) {
	a.input = input
	return &awssdk.UpdateItemOutput{}, a.err
}

func (a *loginAttemptAPI) Query(context.Context, *awssdk.QueryInput, ...func(*awssdk.Options)) (*awssdk.QueryOutput, error) {
	return &awssdk.QueryOutput{}, nil
}

func (a *loginAttemptAPI) TransactWriteItems(context.Context, *awssdk.TransactWriteItemsInput, ...func(*awssdk.Options)) (*awssdk.TransactWriteItemsOutput, error) {
	return &awssdk.TransactWriteItemsOutput{}, nil
}
