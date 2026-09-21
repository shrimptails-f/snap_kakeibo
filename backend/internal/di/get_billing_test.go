package di

import (
	"testing"

	"snap_kakeibo/backend/internal/billing/library/settings"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestNewGetBillingContainerResolvesUsecases(t *testing.T) {
	t.Parallel()
	cfg := settings.Config{BillingsTable: "billings", BillingDetailsTable: "billing-details", JWTSecretParameter: "/test/jwt-secret", Stage: "test"}
	container, err := NewGetBillingContainer(cfg, aws.Config{Region: "ap-northeast-1"}, oswrapper.New(), logger.NewNop())
	if err != nil {
		t.Fatalf("NewGetBillingContainer() error = %v", err)
	}
	get, err := ResolveGetBillingUsecase(container)
	if err != nil || get == nil {
		t.Fatalf("ResolveGetBillingUsecase() = %v, %v", get, err)
	}
	check, err := ResolveAuthCheckUsecase(container)
	if err != nil || check == nil {
		t.Fatalf("ResolveAuthCheckUsecase() = %v, %v", check, err)
	}
}
