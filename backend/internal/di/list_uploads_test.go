package di

import (
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/upload/library/settings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewListUploadsContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.ListUploadsConfig{UploadHistoriesTable: "upload-histories", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewListUploadsContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewListUploadsContainer() error = %v", err)
	}
	list, err := ResolveListUploadsUsecase(container)
	if err != nil || list == nil {
		t.Fatalf("ResolveListUploadsUsecase() = %v, %v", list, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
