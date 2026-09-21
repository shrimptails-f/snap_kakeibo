// Package queue は analyze キューのメッセージ本文を解析ジョブに変換する。
//
// メッセージは 2 形式ある。
//   - retry-analysis が送る JSON({"user_id","analysis_request_id","attempt","trigger":"RETRY"})。画像はレシートバケットの固定キー
//   - S3 の ObjectCreated 通知(S3Event)。receipts/<user_id>/<analysis_request_id>/<file> 以外のキーは無視する
package queue

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"snap_kakeibo/backend/internal/analysis/domain"

	"github.com/aws/aws-lambda-go/events"
)

// キーの構成。upload Lambda が発行する署名付き URL と揃える。
const (
	keyPrefix    = "receipts"
	originalFile = "original.jpg"
	keySegments  = 4 // receipts/<user_id>/<analysis_request_id>/<file>
)

// retryMessage は retry-analysis が送るメッセージ本文。
type retryMessage struct {
	UserID            string `json:"user_id"`
	AnalysisRequestID string `json:"analysis_request_id"`
	Attempt           int    `json:"attempt"`
	Trigger           string `json:"trigger"`
}

// DecodeJobs は SQS メッセージ本文から解析ジョブを取り出す。
// receiptBucket は RETRY 形式のジョブの画像バケット(S3 通知は通知自身がバケットを運ぶ)。
// どちらの形式としても読めなければ error。S3 通知に対象外のキーしかなければ空のスライスを返す。
func DecodeJobs(body, receiptBucket string) ([]domain.AnalysisJob, error) {
	var retry retryMessage
	if json.Unmarshal([]byte(body), &retry) == nil && retry.Trigger == string(domain.TriggerRetry) && retry.UserID != "" && retry.AnalysisRequestID != "" && retry.Attempt > 0 {
		return []domain.AnalysisJob{{
			UserID:            retry.UserID,
			AnalysisRequestID: retry.AnalysisRequestID,
			Attempt:           retry.Attempt,
			Trigger:           domain.TriggerRetry,
			Bucket:            receiptBucket,
			Key:               OriginalKey(retry.UserID, retry.AnalysisRequestID),
		}}, nil
	}
	var event events.S3Event
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return nil, fmt.Errorf("decode queue message: %w", err)
	}
	jobs := make([]domain.AnalysisJob, 0, len(event.Records))
	for _, r := range event.Records {
		// S3 通知のキーは URL エンコードされている
		key, err := url.QueryUnescape(r.S3.Object.Key)
		if err != nil {
			return nil, fmt.Errorf("decode s3 object key: %w", err)
		}
		userID, requestID, ok := IDsFromKey(key)
		if !ok {
			continue
		}
		jobs = append(jobs, domain.AnalysisJob{UserID: userID, AnalysisRequestID: requestID, Attempt: 1, Trigger: domain.TriggerS3, Bucket: r.S3.Bucket.Name, Key: key})
	}
	return jobs, nil
}

// OriginalKey はアップロードされた元画像のキー(receipts/<user_id>/<analysis_request_id>/original.jpg)を返す。
func OriginalKey(userID, requestID string) string {
	return strings.Join([]string{keyPrefix, userID, requestID, originalFile}, "/")
}

// IDsFromKey は receipts/<user_id>/<analysis_request_id>/<file> 形式のキーから user_id と analysis_request_id を取り出す。
// 形式が違えば ok=false。
func IDsFromKey(key string) (userID, requestID string, ok bool) {
	p := strings.Split(key, "/")
	if len(p) != keySegments || p[0] != keyPrefix || p[1] == "" || p[2] == "" || p[3] == "" {
		return "", "", false
	}
	return p[1], p[2], true
}
