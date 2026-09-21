package di

import (
	"testing"

	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewAuthCheckContainerResolvesUsecaseInterface(t *testing.T) {
	t.Parallel()
	container, err := NewAuthCheckContainer(settings.Config{JWTSecretParameter: "/test/jwt-secret", Stage: "test"}, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewAuthCheckContainer() error = %v", err)
	}
	usecase, err := ResolveAuthCheckUsecase(container)
	if err != nil {
		t.Fatalf("ResolveAuthCheckUsecase() error = %v", err)
	}
	if usecase == nil {
		t.Fatal("ResolveAuthCheckUsecase() returned nil")
	}
}
