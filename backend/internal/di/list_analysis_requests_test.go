package di

import (
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewListAnalysisRequestsContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.ListAnalysisRequestsConfig{AnalysisRequestsTable: "analysis-requests", ExpensesTable: "expenses", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewListAnalysisRequestsContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewListAnalysisRequestsContainer() error = %v", err)
	}
	list, err := ResolveListAnalysisRequestsUsecase(container)
	if err != nil || list == nil {
		t.Fatalf("ResolveListAnalysisRequestsUsecase() = %v, %v", list, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
