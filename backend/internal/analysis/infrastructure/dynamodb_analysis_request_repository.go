package infrastructure

import (
	"context"
	"strconv"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// maxErrorMessageRunes は error_message に残す長さ。
const maxErrorMessageRunes = 500

// DynamoDBAnalysisRequestRepository は analysis_requests の状態遷移を条件付き UpdateItem で行う。
// キーの形式(USER#<user_id> / ANALYSIS_REQUEST#<analysis_request_id>)は keys.go で定義し、upload / list-analysis-requests と揃える。
// 状態の値は domain.AnalysisStatus と同じ語彙で、upload / retry-analysis / list-analysis-requests と揃える。
type DynamoDBAnalysisRequestRepository struct {
	Table *libdynamodb.Table
}

var _ application.AnalysisRequestRepository = DynamoDBAnalysisRequestRepository{}

// MarkAnalyzing は UPLOADING / ANALYZING かつ attempt が一致するときだけ ANALYZING にする。
// 条件不一致(既に終端状態、または別の attempt)は false を返し、error にしない。
func (r DynamoDBAnalysisRequestRepository) MarkAnalyzing(ctx context.Context, job domain.AnalysisJob, now time.Time) (bool, error) {
	_, err := r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                      requestKey(job),
		UpdateExpression:         aws.String("SET #status=:analyzing, updated_at=:now"),
		ConditionExpression:      aws.String("#status IN (:uploading,:analyzing) AND attempt=:attempt"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":uploading": stringValue(string(domain.AnalysisStatusUploading)),
			":analyzing": stringValue(string(domain.AnalysisStatusAnalyzing)),
			":attempt":   numberValue(int64(job.Attempt)),
			":now":       stringValue(formatTime(now)),
		},
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return false, nil
	}
	return err == nil, err
}

// MarkFailed は ANALYZING の解析依頼を FAILED にし、error_code / error_message / failed_at を記録する。
// 条件不一致は既に別の処理が終端状態にしたとみなし、成功として扱う。
func (r DynamoDBAnalysisRequestRepository) MarkFailed(ctx context.Context, job domain.AnalysisJob, reason domain.FailureReason, rawResultKey string, now time.Time) error {
	values := terminalValues(job, string(domain.AnalysisStatusFailed), rawResultKey, now)
	values[":code"] = stringValue(reason.Code())
	values[":message"] = stringValue(truncateRunes(reason.SafeMessage(), maxErrorMessageRunes))
	expr := "SET #status=:status,error_code=:code,error_message=:message,failed_at=:now,updated_at=:now"
	if rawResultKey != "" {
		expr += ",raw_result_s3_key=:raw_key"
	}
	return r.markTerminal(ctx, job, expr, values)
}

// MarkNoData は ANALYZING の解析依頼を NO_DATA にし、過去の失敗情報を消す。条件不一致は成功として扱う。
func (r DynamoDBAnalysisRequestRepository) MarkNoData(ctx context.Context, job domain.AnalysisJob, rawResultKey string, now time.Time) error {
	values := terminalValues(job, string(domain.AnalysisStatusNoData), rawResultKey, now)
	expr := "SET #status=:status,updated_at=:now"
	if rawResultKey != "" {
		expr += ",raw_result_s3_key=:raw_key"
	}
	expr += " REMOVE error_code,error_message,failed_at"
	return r.markTerminal(ctx, job, expr, values)
}

func (r DynamoDBAnalysisRequestRepository) markTerminal(ctx context.Context, job domain.AnalysisJob, expr string, values map[string]ddbtypes.AttributeValue) error {
	_, err := r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                       requestKey(job),
		UpdateExpression:          aws.String(expr),
		ConditionExpression:       aws.String(terminalCondition),
		ExpressionAttributeNames:  map[string]string{"#status": "status"},
		ExpressionAttributeValues: values,
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return nil
	}
	return err
}

// terminalCondition は ANALYZING から終端状態へ遷移するときの条件。同じジョブの重複配信で結果を上書きしない。
const terminalCondition = "#status=:analyzing AND attempt=:attempt"

func requestKey(job domain.AnalysisJob) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(job.UserID)), "SK": stringValue(AnalysisRequestSK(job.AnalysisRequestID))}
}

// terminalValues は終端状態への遷移で共通の式の値を返す。rawResultKey は空でなければ :raw_key に載せる。
func terminalValues(job domain.AnalysisJob, status, rawResultKey string, now time.Time) map[string]ddbtypes.AttributeValue {
	values := map[string]ddbtypes.AttributeValue{
		":status":    stringValue(status),
		":analyzing": stringValue(string(domain.AnalysisStatusAnalyzing)),
		":attempt":   numberValue(int64(job.Attempt)),
		":now":       stringValue(formatTime(now)),
	}
	if rawResultKey != "" {
		values[":raw_key"] = stringValue(rawResultKey)
	}
	return values
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }

func numberValue(v int64) ddbtypes.AttributeValue {
	return &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
