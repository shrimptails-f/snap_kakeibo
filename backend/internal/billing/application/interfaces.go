package application

import (
	"context"

	"snap_kakeibo/backend/internal/billing/domain"
)

// BillingFinder は利用者の請求を billing_id で引く。
type BillingFinder interface {
	// FindByID は請求を返す。利用者の請求に無ければ ErrBillingNotFound。
	FindByID(ctx context.Context, userID, billingID string) (domain.Billing, error)
}

// BillingDetailLister は請求の明細を引く。
type BillingDetailLister interface {
	// ListByBilling は明細を detail_id の昇順(採番順)で返す。該当が無ければ空のスライス。
	ListByBilling(ctx context.Context, userID, billingID string) ([]domain.BillingDetail, error)
}
