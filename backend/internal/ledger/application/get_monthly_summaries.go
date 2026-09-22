package application

import (
	"context"
	"fmt"
	"strings"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/logger"
)

type GetMonthlySummariesInput struct{ UserID string }
type GetMonthlySummariesOutput struct{ Summaries []MonthlySummaryItem }

type GetMonthlySummariesUsecaseInterface interface {
	Get(ctx context.Context, in GetMonthlySummariesInput) (GetMonthlySummariesOutput, error)
}

type GetMonthlySummariesUsecase struct{ Summaries MonthlySummaryLister }

func NewGetMonthlySummariesUsecase(summaries MonthlySummaryLister) GetMonthlySummariesUsecaseInterface {
	return &GetMonthlySummariesUsecase{Summaries: summaries}
}

func (u *GetMonthlySummariesUsecase) Get(ctx context.Context, in GetMonthlySummariesInput) (GetMonthlySummariesOutput, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return GetMonthlySummariesOutput{}, ErrInvalidInput
	}
	userID, ok := common.NewUserID(in.UserID)
	if !ok {
		return GetMonthlySummariesOutput{}, ErrInvalidInput
	}
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID))
	summaries, err := u.Summaries.List(ctx, userID)
	if err != nil {
		return GetMonthlySummariesOutput{}, fmt.Errorf("list monthly summaries: %w", err)
	}
	return GetMonthlySummariesOutput{Summaries: summaries}, nil
}
