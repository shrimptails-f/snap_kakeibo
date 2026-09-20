package di

import (
	"testing"

	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewAuthRefreshContainerResolvesUsecaseInterface(t *testing.T) {
	t.Parallel()
	container, err := NewAuthRefreshContainer(settings.Config{
		UsersTable:         "users",
		RefreshTokensTable: "refresh-tokens",
		JWTSecret:          "test-secret",
		Stage:              "test",
	}, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewAuthRefreshContainer() error = %v", err)
	}
	usecase, err := ResolveAuthRefreshUsecase(container)
	if err != nil {
		t.Fatalf("ResolveAuthRefreshUsecase() error = %v", err)
	}
	if usecase == nil {
		t.Fatal("ResolveAuthRefreshUsecase() returned nil")
	}
}
