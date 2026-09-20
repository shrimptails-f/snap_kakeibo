// Package infrastructure はアップロードの application が定義した interface の DynamoDB / S3 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
	"time"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// itemType は upload_histories.type の値。
const itemType = "UPLOAD_HISTORY"

// uploadHistoryItem は upload_histories の項目。list-uploads / analyze-receipt が読む属性名と揃える。
type uploadHistoryItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	GSI1PK      string `dynamodbav:"GSI1PK"`
	GSI1SK      string `dynamodbav:"GSI1SK"`
	Type        string `dynamodbav:"type"`
	UploadID    string `dynamodbav:"upload_id"`
	Status      string `dynamodbav:"status"`
	Attempt     int    `dynamodbav:"attempt"`
	S3Key       string `dynamodbav:"s3_key"`
	FileName    string `dynamodbav:"file_name"`
	ContentType string `dynamodbav:"content_type"`
	YearMonth   string `dynamodbav:"year_month"`
	ExpiresAt   string `dynamodbav:"expires_at"`
	CreatedAt   string `dynamodbav:"created_at"`
	UpdatedAt   string `dynamodbav:"updated_at"`
}

// DynamoDBUploadHistoryRepository は upload_histories に履歴を新規登録する。
type DynamoDBUploadHistoryRepository struct {
	Table *libdynamodb.Table
}

var _ application.UploadHistoryRepository = DynamoDBUploadHistoryRepository{}

// Save は条件付き PutItem で履歴を登録する。同じキーが既にあれば ErrUploadAlreadyExists。
func (r DynamoDBUploadHistoryRepository) Save(ctx context.Context, history domain.UploadHistory) error {
	item, err := attributevalue.MarshalMap(newUploadHistoryItem(history))
	if err != nil {
		return fmt.Errorf("marshal upload history: %w", err)
	}
	_, err = r.Table.PutItem(ctx, &awssdk.PutItemInput{
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return application.ErrUploadAlreadyExists
	}
	return err
}

func newUploadHistoryItem(h domain.UploadHistory) uploadHistoryItem {
	createdAt := formatTime(h.CreatedAt)
	return uploadHistoryItem{
		PK:          UserPK(h.UserID),
		SK:          UploadSK(h.UploadID),
		GSI1PK:      UploadMonthPK(h.UserID, h.YearMonth),
		GSI1SK:      UploadMonthSK(createdAt, h.UploadID),
		Type:        itemType,
		UploadID:    h.UploadID,
		Status:      string(h.Status),
		Attempt:     h.Attempt,
		S3Key:       h.S3Key,
		FileName:    h.FileName,
		ContentType: h.ContentType,
		YearMonth:   h.YearMonth,
		ExpiresAt:   formatTime(h.ExpiresAt),
		CreatedAt:   createdAt,
		UpdatedAt:   formatTime(h.UpdatedAt),
	}
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
