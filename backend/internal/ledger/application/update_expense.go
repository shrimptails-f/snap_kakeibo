package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

type UpdateExpenseDetailInput struct {
	DetailID string
	Name     string
	Amount   int64
	Quantity int64
	Category string
}

type UpdateExpenseInput struct {
	UserID           string
	ExpenseID        string
	StoreName        string
	PurchaseDate     string
	AdjustmentAmount int64
	Details          []UpdateExpenseDetailInput
}

type UpdateExpenseOutput struct {
	Expense   domain.Expense
	UpdatedAt time.Time
}

type UpdateExpenseUsecaseInterface interface {
	Update(ctx context.Context, in UpdateExpenseInput) (UpdateExpenseOutput, error)
}

type UpdateExpenseUsecase struct {
	Expenses ExpenseRepository
	Rebuild  RebuildMonthlySummaryUsecaseInterface
	Clock    timewrapper.Interface
	IDs      IDGenerator
}

func NewUpdateExpenseUsecase(expenses ExpenseRepository, rebuild RebuildMonthlySummaryUsecaseInterface, clock timewrapper.Interface, ids IDGenerator) UpdateExpenseUsecaseInterface {
	return &UpdateExpenseUsecase{Expenses: expenses, Rebuild: rebuild, Clock: clock, IDs: ids}
}

func (u *UpdateExpenseUsecase) Update(ctx context.Context, in UpdateExpenseInput) (UpdateExpenseOutput, error) {
	userID, ok := common.NewUserID(in.UserID)
	if !ok {
		return UpdateExpenseOutput{}, ErrInvalidInput
	}
	expenseID, ok := common.NewExpenseID(in.ExpenseID)
	if !ok {
		return UpdateExpenseOutput{}, ErrInvalidInput
	}
	purchaseDate, err := common.NewPurchaseDate(in.PurchaseDate)
	if err != nil || strings.TrimSpace(in.StoreName) == "" {
		return UpdateExpenseOutput{}, ErrInvalidInput
	}
	ctx = logger.ContextWith(ctx, logger.UserID(userID.String()), logger.ExpenseID(expenseID.String()))
	expense, err := u.Expenses.FindByID(ctx, userID, expenseID)
	if err != nil {
		return UpdateExpenseOutput{}, err
	}
	if len(in.Details) < 1 || len(in.Details) > 50 {
		return UpdateExpenseOutput{}, ErrInvalidInput
	}
	previousDetails := expense.Details()
	existingIDs := make(map[domain.ExpenseDetailID]struct{}, len(previousDetails))
	for _, detail := range previousDetails {
		existingIDs[detail.ID()] = struct{}{}
	}
	requested := make(map[domain.ExpenseDetailID]UpdateExpenseDetailInput, len(in.Details))
	var additions []UpdateExpenseDetailInput
	for _, raw := range in.Details {
		if raw.DetailID == "" {
			additions = append(additions, raw)
			continue
		}
		id, ok := domain.NewExpenseDetailID(raw.DetailID)
		if !ok {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		if _, exists := existingIDs[id]; !exists {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		if _, exists := requested[id]; exists {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		requested[id] = raw
	}
	oldMonth := expense.PurchaseDate().YearMonth()
	if expense.StoreName() != in.StoreName {
		expense.ChangeStoreName(in.StoreName)
	}
	if expense.PurchaseDate() != purchaseDate {
		if err := expense.ChangePurchaseDate(purchaseDate); err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
	}
	if expense.AdjustmentAmount().Yen() != in.AdjustmentAmount {
		if err := expense.AdjustAmount(domain.NewAdjustmentAmount(in.AdjustmentAmount)); err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
	}
	for _, existing := range expense.Details() {
		raw, exists := requested[existing.ID()]
		if !exists {
			if err := expense.RemoveDetail(existing.ID()); err != nil {
				return UpdateExpenseOutput{}, ErrInvalidInput
			}
			continue
		}
		amount, err := common.NewDetailAmount(raw.Amount)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		quantity, err := common.NewQuantity(raw.Quantity)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		category, err := common.NewCategory(raw.Category)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		if existing.Name() != raw.Name {
			if err := expense.RenameDetail(existing.ID(), raw.Name); err != nil {
				return UpdateExpenseOutput{}, ErrInvalidInput
			}
		}
		if existing.Amount() != amount || existing.Quantity() != quantity {
			if err := expense.ChangeDetailAmount(existing.ID(), amount, quantity); err != nil {
				return UpdateExpenseOutput{}, ErrInvalidInput
			}
		}
		if existing.Category() != category {
			if err := expense.ChangeDetailCategory(existing.ID(), category); err != nil {
				return UpdateExpenseOutput{}, ErrInvalidInput
			}
		}
	}
	for _, raw := range additions {
		amount, err := common.NewDetailAmount(raw.Amount)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		quantity, err := common.NewQuantity(raw.Quantity)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		category, err := common.NewCategory(raw.Category)
		if err != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		id, err := u.IDs.NewID()
		if err != nil {
			return UpdateExpenseOutput{}, fmt.Errorf("generate detail id: %w", err)
		}
		detailID, ok := domain.NewExpenseDetailID(id)
		if !ok {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
		detail, err := domain.NewExpenseDetail(detailID, raw.Name, amount, quantity, category, domain.CategorySourceUser)
		if err != nil || expense.AddDetail(detail) != nil {
			return UpdateExpenseOutput{}, ErrInvalidInput
		}
	}
	if len(expense.Details()) != len(in.Details) {
		return UpdateExpenseOutput{}, ErrInvalidInput
	}
	updatedAt := u.Clock.Now()
	if err := u.Expenses.Save(ctx, expense, previousDetails, updatedAt); err != nil {
		if errors.Is(err, ErrExpenseNotFound) {
			return UpdateExpenseOutput{}, ErrExpenseNotFound
		}
		return UpdateExpenseOutput{}, err
	}
	newMonth := expense.PurchaseDate().YearMonth()
	if _, err := u.Rebuild.Rebuild(ctx, RebuildMonthlySummaryInput{UserID: userID.String(), YearMonth: oldMonth.String()}); err != nil {
		return UpdateExpenseOutput{}, err
	}
	if newMonth != oldMonth {
		if _, err := u.Rebuild.Rebuild(ctx, RebuildMonthlySummaryInput{UserID: userID.String(), YearMonth: newMonth.String()}); err != nil {
			return UpdateExpenseOutput{}, err
		}
	}
	return UpdateExpenseOutput{Expense: expense, UpdatedAt: updatedAt}, nil
}
