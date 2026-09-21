package di

import (
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewRetryAnalysisContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.RetryAnalysisConfig{AnalysisRequestsTable: "analysis-requests", AnalyzeQueueURL: "http://q/analyze", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewRetryAnalysisContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewRetryAnalysisContainer() error = %v", err)
	}
	retry, err := ResolveRetryAnalysisUsecase(container)
	if err != nil || retry == nil {
		t.Fatalf("ResolveRetryAnalysisUsecase() = %v, %v", retry, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
