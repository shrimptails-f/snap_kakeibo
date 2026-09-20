package di

import (
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewUploadContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.Config{UploadHistoriesTable: "upload-histories", ReceiptBucket: "receipts", JWTSecret: "test-secret", Stage: "test"}
	container, err := NewUploadContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewUploadContainer() error = %v", err)
	}
	upload, err := ResolveUploadUsecase(container)
	if err != nil || upload == nil {
		t.Fatalf("ResolveUploadUsecase() = %v, %v", upload, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
