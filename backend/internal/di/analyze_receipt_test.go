package di

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/library/settings"
	"snap_kakeibo/backend/internal/library/awstest"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/ssm/ssmtest"
)

func analyzeReceiptConfig(parameter string) settings.Config {
	return settings.Config{
		AnalysisRequestsTable: "analysis-requests",
		ExpensesTable:         "expenses",
		ExpenseDetailsTable:   "expense-details",
		MonthlySummariesTable: "monthly-summaries",
		ReceiptBucket:         "receipts",
		OpenAIAPIKeyParameter: parameter,
		OpenAIModel:           "gpt-5-mini",
		OpenAIReasoningEffort: "low",
		ImageMaxEdge:          2048,
		Stage:                 "test",
	}
}

// TestNewAnalyzeReceiptContainerResolvesUsecaseInterface は OpenAI の API key を Floci の SSM から起動時に取得して
// ユースケースを組み立てられることを確認する。STAGE が local / ci のときだけ動く。
func TestNewAnalyzeReceiptContainerResolvesUsecaseInterface(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)

	name := ssmtest.PutSecureString(t, env.Config, "di-openai-api-key", "sk-test")

	container, err := NewAnalyzeReceiptContainer(analyzeReceiptConfig(name), env.Config, oswrapper.New(), logger.NewNop())
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

// TestNewAnalyzeReceiptContainerFailsWithoutAPIKey は API key のパラメータが無ければ起動時に失敗することを確認する。
func TestNewAnalyzeReceiptContainerFailsWithoutAPIKey(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	container, err := NewAnalyzeReceiptContainer(analyzeReceiptConfig("/"+awstest.ResourceName("di-missing-api-key")), env.Config, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewAnalyzeReceiptContainer() error = %v", err)
	}
	if _, err := ResolveAnalyzeReceiptUsecase(container); err == nil {
		t.Fatal("ResolveAnalyzeReceiptUsecase() error = nil, want a missing parameter error")
	}
}
