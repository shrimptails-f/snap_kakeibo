// Package receipt_test は画像受付・解析・家計簿を横断する業務シナリオテストを提供する。
package receipt_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	analysisapp "snap_kakeibo/backend/internal/analysis/application"
	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	analysisinfra "snap_kakeibo/backend/internal/analysis/infrastructure"
	analysisimage "snap_kakeibo/backend/internal/analysis/library/image"
	ledgerapp "snap_kakeibo/backend/internal/ledger/application"
	ledgerinfra "snap_kakeibo/backend/internal/ledger/infrastructure"
	"snap_kakeibo/backend/internal/library/awstest"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"
	"snap_kakeibo/backend/internal/library/logger"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	"snap_kakeibo/backend/internal/library/timewrapper"
	uploadapp "snap_kakeibo/backend/internal/upload/application"
	uploadinfra "snap_kakeibo/backend/internal/upload/infrastructure"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var scenarioNow = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

// TestReceiptUploadAnalysisAndRetrieval は署名済みフォーム発行 → 画像 POST → 解析 → 保存 → 支出取得を、
// OpenAI 以外は本番と同じ application / infrastructure 実装と Floci の一時リソースで実行する。
func TestReceiptUploadAnalysisAndRetrieval(t *testing.T) {
	t.Parallel()

	env := dynamodbtest.Connect(t)
	analysisRequests := env.CreateTableWithPrefix(t, "receipt-flow", libdynamodb.AnalysisRequestsSchema)
	expenses := env.CreateTableWithPrefix(t, "receipt-flow", libdynamodb.ExpensesSchema)
	expenseDetails := env.CreateTableWithPrefix(t, "receipt-flow", libdynamodb.ExpenseDetailsSchema)
	monthlySummaries := env.CreateTableWithPrefix(t, "receipt-flow", libdynamodb.MonthlySummariesSchema)
	bucketName := createBucket(t, env.Config)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	log := logger.NewNop()
	clock := timewrapper.NewFixed(scenarioNow)
	s3Client := libs3.New(env.Config, log)
	bucket := s3Client.Bucket(bucketName)

	upload := uploadapp.NewCreateUploadUsecase(
		uploadinfra.DynamoDBAnalysisRequestRepository{Table: analysisRequests},
		uploadinfra.S3UploadPresigner{Bucket: bucket},
		&sequenceIDs{values: []string{"request-receipt-flow"}},
		clock,
	)
	uploaded, err := upload.Create(ctx, uploadapp.CreateUploadInput{
		UserID: "scenario-user", FileName: "receipt.png", ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if uploaded.S3Key != "receipts/scenario-user/request-receipt-flow/original.jpg" || uploaded.PostForm.URL == "" {
		t.Fatalf("upload output = %+v", uploaded)
	}
	postPresigned(t, ctx, uploaded.PostForm, receiptPNG(t))

	storage := analysisinfra.S3ReceiptStorage{Client: s3Client, Results: bucket}
	analyze := analysisapp.NewAnalyzeReceiptUsecase(
		storage,
		analysisimage.Resizer{MaxEdge: 2048},
		receiptAnalyzer{},
		storage,
		analysisinfra.DynamoDBAnalysisRequestRepository{Table: analysisRequests},
		analysisinfra.DynamoDBExpenseRegistrar{
			Client: env.Client, AnalysisRequests: analysisRequests, Expenses: expenses,
			ExpenseDetails: expenseDetails, MonthlySummaries: monthlySummaries,
		},
		&sequenceIDs{values: []string{"expense-receipt-flow", "detail-milk", "detail-bread"}},
		clock,
		log,
	)
	analyzed, err := analyze.Analyze(ctx, analysisapp.AnalyzeReceiptInput{Job: analysisdomain.AnalysisJob{
		UserID: "scenario-user", AnalysisRequestID: uploaded.AnalysisRequestID, Attempt: 1,
		Trigger: analysisdomain.TriggerS3, Bucket: bucketName, Key: uploaded.S3Key,
	}})
	if err != nil {
		t.Fatalf("analyze receipt: %v", err)
	}
	if analyzed.Outcome != analysisdomain.OutcomeSucceeded || analyzed.ExpenseID != "expense-receipt-flow" {
		t.Fatalf("analysis output = %+v", analyzed)
	}
	raw, err := bucket.GetBytes(ctx, analyzed.RawResultKey, 1024)
	if err != nil || string(raw) != `{"id":"response-receipt-flow"}` {
		t.Fatalf("stored raw result = %q, err = %v", raw, err)
	}

	getExpense := ledgerapp.NewGetExpenseUsecase(ledgerinfra.DynamoDBExpenseRepository{
		Expenses: expenses, ExpenseDetails: expenseDetails,
	}, ledgerinfra.S3ReceiptImageURLPresigner{Bucket: bucket})
	got, err := getExpense.Get(ctx, ledgerapp.GetExpenseInput{UserID: "scenario-user", ExpenseID: analyzed.ExpenseID})
	if err != nil {
		t.Fatalf("get expense: %v", err)
	}
	if got.ImageURL == "" {
		t.Error("get expense image URL is empty")
	}
	expense := got.Expense
	if expense.ID().String() != "expense-receipt-flow" || expense.SourceRequestID().String() != uploaded.AnalysisRequestID ||
		expense.StoreName() != "テストスーパー" || expense.PurchaseDate().String() != "2026-09-20" ||
		expense.ReadAmount().Yen() != 350 || expense.RecordedAmount().Yen() != 350 {
		t.Errorf("expense = %+v", expense)
	}
	details := expense.Details()
	detailAmounts := make(map[string]int64, len(details))
	for _, detail := range details {
		detailAmounts[detail.Name()] = detail.Amount().Yen()
	}
	if len(details) != 2 || detailAmounts["牛乳"] != 200 || detailAmounts["パン"] != 150 {
		t.Errorf("expense details = %+v", details)
	}
}

type sequenceIDs struct {
	values []string
	next   int
}

func (s *sequenceIDs) NewID() (string, error) {
	if s.next >= len(s.values) {
		return "", fmt.Errorf("test ID sequence exhausted")
	}
	id := s.values[s.next]
	s.next++
	return id, nil
}

type receiptAnalyzer struct{}

func (receiptAnalyzer) Analyze(_ context.Context, data []byte) (analysisapp.AnalyzerResponse, error) {
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return analysisapp.AnalyzerResponse{}, fmt.Errorf("analyzer received non-JPEG image: %w", err)
	}
	store, date, amount := "テストスーパー", "2026-09-20", int64(350)
	return analysisapp.AnalyzerResponse{
		ResponseID: "response-receipt-flow",
		Raw:        []byte(`{"id":"response-receipt-flow"}`),
		Reading: analysisdomain.ReceiptReading{
			StoreName: &store, PurchaseDate: &date, ReadAmount: &amount,
			Details: []analysisdomain.ReadDetail{
				{Name: "牛乳", Amount: 200, Quantity: 1, Category: "food"},
				{Name: "パン", Amount: 150, Quantity: 1, Category: "food"},
			},
		},
	}, nil
}

func receiptPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 * x), G: uint8(60 * y), B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

func postPresigned(t *testing.T, ctx context.Context, form uploadapp.UploadForm, image []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range form.Fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "receipt.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(image); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, form.URL, &body)
	if err != nil {
		t.Fatalf("create presigned POST request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST presigned URL: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		t.Fatalf("POST presigned URL: status %d: %s", resp.StatusCode, message)
	}
}

func createBucket(t *testing.T, cfg aws.Config) string {
	t.Helper()
	name := awstest.ResourceName("receipt-flow")
	client := awss3.NewFromConfig(cfg, func(options *awss3.Options) { options.UsePathStyle = true })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{
		Bucket:                    aws.String(name),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(cfg.Region)},
	}); err != nil {
		t.Fatalf("create S3 bucket %s: %v", name, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		listed, err := client.ListObjectsV2(cleanupCtx, &awss3.ListObjectsV2Input{Bucket: aws.String(name)})
		if err == nil {
			for _, object := range listed.Contents {
				_, _ = client.DeleteObject(cleanupCtx, &awss3.DeleteObjectInput{Bucket: aws.String(name), Key: object.Key})
			}
		}
		if _, err := client.DeleteBucket(cleanupCtx, &awss3.DeleteBucketInput{Bucket: aws.String(name)}); err != nil {
			t.Errorf("delete S3 bucket %s: %v", name, err)
		}
	})
	return name
}
