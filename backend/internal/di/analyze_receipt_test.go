package di

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewAnalyzeReceiptContainerResolvesUsecaseInterface(t *testing.T) {
	t.Parallel()
	container, err := NewAnalyzeReceiptContainer(settings.Config{
		UploadHistoriesTable:  "upload-histories",
		BillingsTable:         "billings",
		BillingDetailsTable:   "billing-details",
		MonthlySummariesTable: "monthly-summaries",
		ReceiptBucket:         "receipts",
		OpenAIAPIKey:          "sk-test",
		OpenAIModel:           "gpt-5-mini",
		OpenAIReasoningEffort: "low",
		ImageMaxEdge:          2048,
		Stage:                 "test",
	}, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewAnalyzeReceiptContainer() error = %v", err)
	}
	usecase, err := ResolveAnalyzeReceiptUsecase(container)
	if err != nil {
		t.Fatalf("ResolveAnalyzeReceiptUsecase() error = %v", err)
	}
	if usecase == nil {
		t.Fatal("ResolveAnalyzeReceiptUsecase() returned nil")
	}
}
