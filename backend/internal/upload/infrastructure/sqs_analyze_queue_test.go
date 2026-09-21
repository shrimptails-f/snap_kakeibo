package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libsqs "snap_kakeibo/backend/internal/library/sqs"
	"snap_kakeibo/backend/internal/upload/domain"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// fakeSQS は SendMessage の入力を記録し、固定の応答か err を返す。
type fakeSQS struct {
	in  *awssqs.SendMessageInput
	err error
}

func (f *fakeSQS) SendMessage(_ context.Context, in *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	return &awssqs.SendMessageOutput{MessageId: aws.String("msg-1")}, nil
}

func TestEnqueueRetryAnalysisSendsRetryMessageToTheBoundQueue(t *testing.T) {
	t.Parallel()
	api := &fakeSQS{}
	queue := SQSAnalyzeQueue{Queue: libsqs.NewWithAPI(api, nil).Queue("http://q/analyze")}

	if err := queue.EnqueueRetryAnalysis(context.Background(), domain.RetryAnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 2}); err != nil {
		t.Fatalf("EnqueueRetryAnalysis() error = %v", err)
	}
	if got := aws.ToString(api.in.QueueUrl); got != "http://q/analyze" {
		t.Errorf("queue_url = %q", got)
	}
	// analyze-receipt の DecodeJobs が RETRY 形式として読む 4 項目だけを送る
	var body map[string]any
	if err := json.Unmarshal([]byte(aws.ToString(api.in.MessageBody)), &body); err != nil {
		t.Fatalf("body = %q: %v", aws.ToString(api.in.MessageBody), err)
	}
	want := map[string]any{"user_id": "u1", "analysis_request_id": "req1", "attempt": float64(2), "trigger": "RETRY"}
	if len(body) != len(want) {
		t.Errorf("body has %d fields, want %d: %+v", len(body), len(want), body)
	}
	for key, value := range want {
		if body[key] != value {
			t.Errorf("body[%q] = %v, want %v", key, body[key], value)
		}
	}
}

func TestEnqueueRetryAnalysisReturnsSendFailure(t *testing.T) {
	t.Parallel()
	api := &fakeSQS{err: errors.New("boom")}
	queue := SQSAnalyzeQueue{Queue: libsqs.NewWithAPI(api, nil).Queue("http://q/analyze")}
	if err := queue.EnqueueRetryAnalysis(context.Background(), domain.RetryAnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 2}); err == nil {
		t.Fatal("EnqueueRetryAnalysis() error = nil, want send failure")
	}
}
