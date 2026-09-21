package di

import (
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewRetryUploadContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.RetryUploadConfig{UploadHistoriesTable: "upload-histories", AnalyzeQueueURL: "http://q/analyze", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewRetryUploadContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewRetryUploadContainer() error = %v", err)
	}
	retry, err := ResolveRetryUploadUsecase(container)
	if err != nil || retry == nil {
		t.Fatalf("ResolveRetryUploadUsecase() = %v, %v", retry, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
