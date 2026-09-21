// Package infrastructure はアップロードの application が定義した interface の DynamoDB / S3 / SQS 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// itemType は analysis_requests.type の値。
const itemType = "ANALYSIS_REQUEST"

// analysisRequestItem は analysis_requests の項目。analyze-receipt が書く属性名と揃える。
// ExpenseID / ErrorCode / ErrorMessage は analyze-receipt が遷移時に書くので、upload の登録では omitempty で書かない。
type analysisRequestItem struct {
	PK                string `dynamodbav:"PK"`
	SK                string `dynamodbav:"SK"`
	GSI1PK            string `dynamodbav:"GSI1PK"`
	GSI1SK            string `dynamodbav:"GSI1SK"`
	Type              string `dynamodbav:"type"`
	AnalysisRequestID string `dynamodbav:"analysis_request_id"`
	Status            string `dynamodbav:"status"`
	Attempt           int    `dynamodbav:"attempt"`
	S3Key             string `dynamodbav:"s3_key"`
	FileName          string `dynamodbav:"file_name"`
	ContentType       string `dynamodbav:"content_type"`
	YearMonth         string `dynamodbav:"year_month"`
	UploadExpiresAt   string `dynamodbav:"upload_expires_at"`
	CreatedAt         string `dynamodbav:"created_at"`
	UpdatedAt         string `dynamodbav:"updated_at"`
	ExpenseID         string `dynamodbav:"expense_id,omitempty"`
	ErrorCode         string `dynamodbav:"error_code,omitempty"`
	ErrorMsg          string `dynamodbav:"error_message,omitempty"`
}

// DynamoDBAnalysisRequestRepository は analysis_requests への解析依頼の新規登録(upload)、再解析の状態遷移(retry-analysis)、
// 月ごとの一覧(list-analysis-requests)を行う。
type DynamoDBAnalysisRequestRepository struct {
	Table *libdynamodb.Table
}

var (
	_ application.AnalysisRequestRepository = DynamoDBAnalysisRequestRepository{}
	_ application.RetryAnalysisMarker       = DynamoDBAnalysisRequestRepository{}
	_ application.AnalysisRequestLister     = DynamoDBAnalysisRequestRepository{}
)

// Save は条件付き PutItem で解析依頼を登録する。同じキーが既にあれば ErrAnalysisRequestAlreadyExists。
func (r DynamoDBAnalysisRequestRepository) Save(ctx context.Context, request domain.AnalysisRequest) error {
	item, err := attributevalue.MarshalMap(newAnalysisRequestItem(request))
	if err != nil {
		return fmt.Errorf("marshal analysis request: %w", err)
	}
	_, err = r.Table.PutItem(ctx, &awssdk.PutItemInput{
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return application.ErrAnalysisRequestAlreadyExists
	}
	return err
}

// MarkRetrying は status が再解析できるときだけ ANALYZING にして attempt を 1 進め、前回の失敗情報を消す。
// 条件式で status を見るので、解析依頼が無い場合も条件不一致になり ErrAnalysisRequestNotRetryable を返す。
// 進めた後の attempt は ReturnValues: UPDATED_NEW で受け取り、呼び出し側が analyze キューへ載せる。
func (r DynamoDBAnalysisRequestRepository) MarkRetrying(ctx context.Context, userID, requestID string, now time.Time) (int, error) {
	out, err := r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                      analysisRequestKey(userID, requestID),
		UpdateExpression:         aws.String("SET #status=:analyzing, attempt=attempt+:one, updated_at=:now REMOVE error_code, error_message, failed_at"),
		ConditionExpression:      aws.String(retryableCondition()),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":analyzing": stringValue(string(analysisdomain.AnalysisStatusAnalyzing)),
			":failed":    stringValue(string(analysisdomain.AnalysisStatusFailed)),
			":no_data":   stringValue(string(analysisdomain.AnalysisStatusNoData)),
			":one":       numberValue(1),
			":now":       stringValue(formatTime(now)),
		},
		ReturnValues: ddbtypes.ReturnValueUpdatedNew,
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return 0, application.ErrAnalysisRequestNotRetryable
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

// ListByMonth は analysis_request_month_index を GSI1PK(利用者 + 月)で引き、作成日時の降順で返す。
// 1 回の Query の範囲(1MB)だけを返し、ページネーションはしない。個人利用で 1 月分がそれを超えない前提。
func (r DynamoDBAnalysisRequestRepository) ListByMonth(ctx context.Context, userID, yearMonth string) ([]domain.AnalysisRequest, error) {
	out, err := r.Table.Query(ctx, &awssdk.QueryInput{
		IndexName:                 aws.String(libdynamodb.AnalysisRequestMonthIndex),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UserMonthPK(userID, yearMonth))},
		ScanIndexForward:          aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	var items []analysisRequestItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("unmarshal analysis requests: %w", err)
	}
	requests := make([]domain.AnalysisRequest, 0, len(items))
	for _, item := range items {
		request, err := item.toDomain(userID)
		if err != nil {
			return nil, fmt.Errorf("restore analysis request %s: %w", item.AnalysisRequestID, err)
		}
		requests = append(requests, request)
	}
	return requests, nil
}

// retryableCondition は domain.RetryableStatuses(FAILED / NO_DATA / ANALYZING)に対応する条件式。
// 値は MarkRetrying の ExpressionAttributeValues と対にする。
func retryableCondition() string { return "#status IN (:failed, :no_data, :analyzing)" }

func analysisRequestKey(userID, requestID string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID)), "SK": stringValue(AnalysisRequestSK(requestID))}
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }

func numberValue(v int64) ddbtypes.AttributeValue {
	return &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}

func newAnalysisRequestItem(request domain.AnalysisRequest) analysisRequestItem {
	userID, requestID := request.UserID().String(), request.ID().String()
	createdAt := formatTime(request.CreatedAt())
	image := request.Image()
	return analysisRequestItem{
		PK:                UserPK(userID),
		SK:                AnalysisRequestSK(requestID),
		GSI1PK:            UserMonthPK(userID, domain.YearMonth(request.CreatedAt())),
		GSI1SK:            AnalysisRequestMonthSK(createdAt, requestID),
		Type:              itemType,
		AnalysisRequestID: requestID,
		Status:            string(request.Status()),
		Attempt:           request.CurrentAttempt().Int(),
		S3Key:             image.Reference(),
		FileName:          image.FileName(),
		ContentType:       image.ContentType(),
		YearMonth:         domain.YearMonth(request.CreatedAt()),
		UploadExpiresAt:   formatTime(request.UploadExpiresAt()),
		CreatedAt:         createdAt,
		UpdatedAt:         formatTime(request.UpdatedAt()),
	}
}

// toDomain は項目を集約へ復元する。userID は PK から復元せず Query の条件をそのまま使う。
// 状態と付随する値(SUCCEEDED なら expense_id、FAILED なら error_code)の整合は集約の復元時に検証される。
func (i analysisRequestItem) toDomain(userID string) (domain.AnalysisRequest, error) {
	user, _ := common.NewUserID(userID)
	requestID, _ := common.NewAnalysisRequestID(i.AnalysisRequestID)
	image, err := analysisdomain.NewReceiptImage(i.S3Key, i.FileName, i.ContentType)
	if err != nil {
		return domain.AnalysisRequest{}, err
	}
	attempt, _ := analysisdomain.NewAttempt(i.Attempt)
	uploadExpiresAt, err := parseTime("upload_expires_at", i.UploadExpiresAt)
	if err != nil {
		return domain.AnalysisRequest{}, err
	}
	createdAt, err := parseTime("created_at", i.CreatedAt)
	if err != nil {
		return domain.AnalysisRequest{}, err
	}
	updatedAt, err := parseTime("updated_at", i.UpdatedAt)
	if err != nil {
		return domain.AnalysisRequest{}, err
	}
	expenseID, _ := common.NewExpenseID(i.ExpenseID)
	var failureReason *analysisdomain.FailureReason
	if i.ErrorCode != "" {
		reason, err := analysisdomain.NewFailureReason(i.ErrorCode, i.ErrorMsg)
		if err != nil {
			return domain.AnalysisRequest{}, err
		}
		failureReason = &reason
	}
	return analysisdomain.RestoreAnalysisRequest(domain.AnalysisRequestState{
		ID: requestID, UserID: user, Image: image, Status: domain.AnalysisStatus(i.Status),
		CurrentAttempt: attempt, UploadExpiresAt: uploadExpiresAt,
		ExpenseID: expenseID, FailureReason: failureReason,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
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
