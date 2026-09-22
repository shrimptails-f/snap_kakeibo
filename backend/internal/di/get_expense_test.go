package di

import (
	"testing"

	"snap_kakeibo/backend/internal/ledger/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewGetExpenseContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.Config{ExpensesTable: "expenses", ExpenseDetailsTable: "expense-details", ReceiptBucket: "receipts", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewGetExpenseContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewGetExpenseContainer() error = %v", err)
	}
	get, err := ResolveGetExpenseUsecase(container)
	if err != nil || get == nil {
		t.Fatalf("ResolveGetExpenseUsecase() = %v, %v", get, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
