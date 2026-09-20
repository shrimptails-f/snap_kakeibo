package di

import (
	"testing"

	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewAuthLogoutContainerResolvesUsecaseInterface(t *testing.T) {
	container, err := NewAuthLogoutContainer(settings.Config{RefreshTokensTable: "refresh-tokens", Stage: "test"}, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewAuthLogoutContainer() error = %v", err)
	}
	usecase, err := ResolveAuthLogoutUsecase(container)
	if err != nil {
		t.Fatalf("ResolveAuthLogoutUsecase() error = %v", err)
	}
	if usecase == nil {
		t.Fatal("ResolveAuthLogoutUsecase() returned nil")
	}
}
