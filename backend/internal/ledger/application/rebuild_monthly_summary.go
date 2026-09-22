package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

const rebuildAttempts = 3

type RebuildMonthlySummaryInput struct {
	UserID    string
	YearMonth string
}

type RebuildMonthlySummaryOutput struct {
	Summary   domain.MonthlySummary
	UpdatedAt time.Time
}

type RebuildMonthlySummaryUsecaseInterface interface {
	Rebuild(ctx context.Context, in RebuildMonthlySummaryInput) (RebuildMonthlySummaryOutput, error)
}

type RebuildMonthlySummaryUsecase struct {
	Expenses  ExpenseRepository
	Summaries MonthlySummaryRepository
	Clock     timewrapper.Interface
}

func NewRebuildMonthlySummaryUsecase(expenses ExpenseRepository, summaries MonthlySummaryRepository, clock timewrapper.Interface) RebuildMonthlySummaryUsecaseInterface {
	return &RebuildMonthlySummaryUsecase{Expenses: expenses, Summaries: summaries, Clock: clock}
}

func (u *RebuildMonthlySummaryUsecase) Rebuild(ctx context.Context, in RebuildMonthlySummaryInput) (RebuildMonthlySummaryOutput, error) {
	userID, ok := common.NewUserID(in.UserID)
	if !ok {
		return RebuildMonthlySummaryOutput{}, ErrInvalidInput
	}
	month, err := common.NewYearMonth(in.YearMonth)
	if err != nil {
		return RebuildMonthlySummaryOutput{}, ErrInvalidInput
	}
	ctx = logger.ContextWith(ctx, logger.UserID(userID.String()), logger.String("year_month", month.String()))
	for attempt := 0; attempt < rebuildAttempts; attempt++ {
		version, err := u.Summaries.Version(ctx, userID, month)
		if err != nil {
			return RebuildMonthlySummaryOutput{}, err
		}
		expenses, err := u.Expenses.FindByMonth(ctx, userID, month)
		if err != nil {
			return RebuildMonthlySummaryOutput{}, err
		}
		summary, err := domain.RebuildMonthlySummary(userID, month, expenses, version)
		if err != nil {
			return RebuildMonthlySummaryOutput{}, err
		}
		updatedAt := u.Clock.Now()
		if err := u.Summaries.Save(ctx, summary, updatedAt); err != nil {
			if errors.Is(err, ErrConcurrentUpdate) {
				continue
			}
			return RebuildMonthlySummaryOutput{}, err
		}
		summary.Version++
		return RebuildMonthlySummaryOutput{Summary: summary, UpdatedAt: updatedAt}, nil
	}
	return RebuildMonthlySummaryOutput{}, fmt.Errorf("rebuild monthly summary after %d attempts: %w", rebuildAttempts, ErrConcurrentUpdate)
}
