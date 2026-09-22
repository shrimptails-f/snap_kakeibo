package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/domain"
)

const (
	// AnalysisRequestsPageSize は解析履歴の1ページに表示する画像数。
	AnalysisRequestsPageSize = 20
	// AnalysisStalledAfter は解析中を停滞として要対応に含めるまでの時間。
	AnalysisStalledAfter = 30 * time.Minute
)

// AnalysisRequestFilter は解析履歴で選択できる状態区分。
type AnalysisRequestFilter string

const (
	AnalysisRequestFilterAll        AnalysisRequestFilter = "all"
	AnalysisRequestFilterAttention  AnalysisRequestFilter = "attention"
	AnalysisRequestFilterInProgress AnalysisRequestFilter = "in_progress"
	AnalysisRequestFilterSucceeded  AnalysisRequestFilter = "succeeded"
	AnalysisRequestFilterNoData     AnalysisRequestFilter = "no_data"
)

// Valid は外部入力として受け付ける状態区分なら true。
func (f AnalysisRequestFilter) Valid() bool {
	switch f {
	case AnalysisRequestFilterAll, AnalysisRequestFilterAttention, AnalysisRequestFilterInProgress, AnalysisRequestFilterSucceeded, AnalysisRequestFilterNoData:
		return true
	default:
		return false
	}
}

// AnalysisRequestListQuery は repository が月全体を絞り込むための条件。
type AnalysisRequestListQuery struct {
	UserID    string
	YearMonth string
	Filter    AnalysisRequestFilter
	Cursor    string
	PageSize  int
	Now       time.Time
}

// AnalysisRequestPage は解析依頼の1ページ。NextCursor が空なら続きはない。
type AnalysisRequestPage struct {
	Requests   []domain.AnalysisRequest
	NextCursor string
}

// ExpenseSummary は解析履歴へ補足する、支出の最新の店舗名と計上額。
type ExpenseSummary struct {
	ExpenseID      string
	StoreName      string
	RecordedAmount int64
}

// AnalysisRequestListItem は解析依頼と、登録済みの場合に取得できた支出の一覧表示項目。
type AnalysisRequestListItem struct {
	Request        domain.AnalysisRequest
	ExpenseSummary *ExpenseSummary
}

// ListAnalysisRequestsInput は解析履歴の入力。Filter が空なら all として扱う。
type ListAnalysisRequestsInput struct {
	UserID    string
	YearMonth string
	Filter    string
	Cursor    string
}

// ListAnalysisRequestsOutput は解析履歴の1ページ。
type ListAnalysisRequestsOutput struct {
	Items      []AnalysisRequestListItem
	NextCursor string
}

// ListAnalysisRequestsUsecase は利用者の指定月を絞り込み、登録済み支出の表示項目を補って返す。
type ListAnalysisRequestsUsecase struct {
	Requests AnalysisRequestLister
	Expenses AnalysisRequestExpenseReader
	Clock    timewrapper.Interface
}

// ListAnalysisRequestsUsecaseInterface は HTTP 層が解析依頼一覧ユースケースへ依存するための契約。
type ListAnalysisRequestsUsecaseInterface interface {
	List(ctx context.Context, in ListAnalysisRequestsInput) (ListAnalysisRequestsOutput, error)
}

var _ ListAnalysisRequestsUsecaseInterface = (*ListAnalysisRequestsUsecase)(nil)

// NewListAnalysisRequestsUsecase は解析依頼一覧ユースケースを生成する。
func NewListAnalysisRequestsUsecase(requests AnalysisRequestLister, expenses AnalysisRequestExpenseReader, clock timewrapper.Interface) ListAnalysisRequestsUsecaseInterface {
	return &ListAnalysisRequestsUsecase{Requests: requests, Expenses: expenses, Clock: clock}
}

// List は入力を検証し、月全体へ状態フィルターを適用した1ページを返す。
func (u *ListAnalysisRequestsUsecase) List(ctx context.Context, in ListAnalysisRequestsInput) (ListAnalysisRequestsOutput, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return ListAnalysisRequestsOutput{}, ErrInvalidInput
	}
	month, err := common.NewYearMonth(in.YearMonth)
	if err != nil {
		return ListAnalysisRequestsOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	filter := AnalysisRequestFilter(in.Filter)
	if filter == "" {
		filter = AnalysisRequestFilterAll
	}
	if !filter.Valid() {
		return ListAnalysisRequestsOutput{}, ErrInvalidInput
	}

	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.String("year_month", month.String()), logger.String("filter", string(filter)))
	page, err := u.Requests.ListPage(ctx, AnalysisRequestListQuery{
		UserID: in.UserID, YearMonth: month.String(), Filter: filter, Cursor: in.Cursor,
		PageSize: AnalysisRequestsPageSize, Now: u.Clock.Now().UTC(),
	})
	if err != nil {
		return ListAnalysisRequestsOutput{}, fmt.Errorf("list analysis requests: %w", err)
	}

	expenseIDs := make([]string, 0, len(page.Requests))
	for _, request := range page.Requests {
		if expenseID := request.ExpenseID().String(); expenseID != "" {
			expenseIDs = append(expenseIDs, expenseID)
		}
	}
	summaries, err := u.Expenses.FindSummaries(ctx, in.UserID, expenseIDs)
	if err != nil {
		return ListAnalysisRequestsOutput{}, fmt.Errorf("find expense summaries: %w", err)
	}
	items := make([]AnalysisRequestListItem, 0, len(page.Requests))
	for _, request := range page.Requests {
		item := AnalysisRequestListItem{Request: request}
		if summary, ok := summaries[request.ExpenseID().String()]; ok {
			copy := summary
			item.ExpenseSummary = &copy
		}
		items = append(items, item)
	}
	return ListAnalysisRequestsOutput{Items: items, NextCursor: page.NextCursor}, nil
}
