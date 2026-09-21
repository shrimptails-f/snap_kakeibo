package domain

import (
	"errors"
	"testing"
)

func TestPurchaseDateAndYearMonth(t *testing.T) {
	t.Parallel()

	date, err := NewPurchaseDate("2026-09-21")
	if err != nil {
		t.Fatalf("NewPurchaseDate() error = %v", err)
	}
	if got := date.YearMonth().String(); got != "2026-09" {
		t.Errorf("YearMonth() = %q, want 2026-09", got)
	}
	if _, err := NewPurchaseDate("2026-02-30"); !errors.Is(err, ErrInvalidPurchaseDate) {
		t.Errorf("NewPurchaseDate() error = %v, want ErrInvalidPurchaseDate", err)
	}
	if _, err := NewYearMonth("2026-9"); !errors.Is(err, ErrInvalidYearMonth) {
		t.Errorf("NewYearMonth() error = %v, want ErrInvalidYearMonth", err)
	}
}

func TestCategoriesIncludesSocial(t *testing.T) {
	t.Parallel()

	category, err := NewCategory("social")
	if err != nil {
		t.Fatalf("NewCategory() error = %v", err)
	}
	if category != CategorySocial {
		t.Errorf("NewCategory() = %q, want %q", category, CategorySocial)
	}
	if _, err := NewCategory("business_dinner"); !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("NewCategory() error = %v, want ErrInvalidCategory", err)
	}
}
