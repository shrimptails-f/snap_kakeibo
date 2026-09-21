// Package infrastructure はアップロードの application が定義した interface の DynamoDB / S3 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"time"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// itemType は upload_histories.type の値。
const itemType = "UPLOAD_HISTORY"

// uploadHistoryItem は upload_histories の項目。analyze-receipt が書く属性名と揃える。
// BillingID / ErrorCode / ErrorMessage は analyze-receipt が遷移時に書くので、upload の登録では omitempty で書かない。
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
	BillingID   string `dynamodbav:"billing_id,omitempty"`
	ErrorCode   string `dynamodbav:"error_code,omitempty"`
	ErrorMsg    string `dynamodbav:"error_message,omitempty"`
}

// DynamoDBUploadHistoryRepository は upload_histories への履歴の新規登録(upload)、再実行の状態遷移(retry-upload)、
// 月ごとの一覧(list-uploads)を行う。
type DynamoDBUploadHistoryRepository struct {
	Table *libdynamodb.Table
}

var (
	_ application.UploadHistoryRepository = DynamoDBUploadHistoryRepository{}
	_ application.UploadRetryMarker       = DynamoDBUploadHistoryRepository{}
	_ application.UploadHistoryLister     = DynamoDBUploadHistoryRepository{}
)

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

// MarkRetrying は status が再実行できるときだけ ANALYZING にして attempt を 1 進め、前回の失敗情報を消す。
// 条件式で status を見るので、履歴が無い場合も条件不一致になり ErrUploadNotRetryable を返す。
// 進めた後の attempt は ReturnValues: UPDATED_NEW で受け取り、呼び出し側が analyze キューへ載せる。
func (r DynamoDBUploadHistoryRepository) MarkRetrying(ctx context.Context, userID, uploadID string, now time.Time) (int, error) {
	out, err := r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                      uploadHistoryKey(userID, uploadID),
		UpdateExpression:         aws.String("SET #status=:analyzing, attempt=attempt+:one, updated_at=:now REMOVE error_code, error_message, failed_at"),
		ConditionExpression:      aws.String(retryableCondition()),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":analyzing": stringValue(string(domain.StatusAnalyzing)),
			":failed":    stringValue(string(domain.StatusFailed)),
			":no_data":   stringValue(string(domain.StatusNoData)),
			":one":       numberValue(1),
			":now":       stringValue(formatTime(now)),
		},
		ReturnValues: ddbtypes.ReturnValueUpdatedNew,
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return 0, application.ErrUploadNotRetryable
	}
	if err != nil {
		return 0, err
	}
	var updated struct {
		Attempt int `dynamodbav:"attempt"`
	}
	if err := attributevalue.UnmarshalMap(out.Attributes, &updated); err != nil {
		return 0, fmt.Errorf("unmarshal updated attempt: %w", err)
	}
	return updated.Attempt, nil
}

// ListByMonth は upload_month_index を GSI1PK(利用者 + 月)で引き、作成日時の降順で返す。
// 1 回の Query の範囲(1MB)だけを返し、ページネーションはしない。個人利用で 1 月分がそれを超えない前提。
func (r DynamoDBUploadHistoryRepository) ListByMonth(ctx context.Context, userID, yearMonth string) ([]domain.UploadHistory, error) {
	out, err := r.Table.Query(ctx, &awssdk.QueryInput{
		IndexName:                 aws.String(libdynamodb.UploadMonthIndex),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UploadMonthPK(userID, yearMonth))},
		ScanIndexForward:          aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	var items []uploadHistoryItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("unmarshal upload histories: %w", err)
	}
	histories := make([]domain.UploadHistory, 0, len(items))
	for _, item := range items {
		history, err := item.toDomain(userID)
		if err != nil {
			return nil, err
		}
		histories = append(histories, history)
	}
	return histories, nil
}

// retryableCondition は domain.RetryableStatuses(FAILED / NO_DATA / ANALYZING)に対応する条件式。
// 値は MarkRetrying の ExpressionAttributeValues と対にする。
func retryableCondition() string { return "#status IN (:failed, :no_data, :analyzing)" }

func uploadHistoryKey(userID, uploadID string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID)), "SK": stringValue(UploadSK(uploadID))}
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }

func numberValue(v int64) ddbtypes.AttributeValue {
	return &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
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

// toDomain は項目を entity にする。userID は PK から復元せず Query の条件をそのまま使う。
func (i uploadHistoryItem) toDomain(userID string) (domain.UploadHistory, error) {
	expiresAt, err := parseTime("expires_at", i.ExpiresAt)
	if err != nil {
		return domain.UploadHistory{}, err
	}
	createdAt, err := parseTime("created_at", i.CreatedAt)
	if err != nil {
		return domain.UploadHistory{}, err
	}
	updatedAt, err := parseTime("updated_at", i.UpdatedAt)
	if err != nil {
		return domain.UploadHistory{}, err
	}
	return domain.UploadHistory{
		UserID:       userID,
		UploadID:     i.UploadID,
		Status:       domain.Status(i.Status),
		Attempt:      i.Attempt,
		S3Key:        i.S3Key,
		FileName:     i.FileName,
		ContentType:  i.ContentType,
		YearMonth:    i.YearMonth,
		ExpiresAt:    expiresAt,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		BillingID:    i.BillingID,
		ErrorCode:    i.ErrorCode,
		ErrorMessage: i.ErrorMsg,
	}, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// parseTime は formatTime の逆変換。属性が無い(空)ならゼロ値にし、形式が違うときだけ属性名を付けてエラーにする。
func parseTime(attribute, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %s: %w", attribute, err)
	}
	return t, nil
}
