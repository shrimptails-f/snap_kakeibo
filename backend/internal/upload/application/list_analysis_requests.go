package application

import (
	"context"
	"fmt"
	"strings"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/upload/domain"
)

// ListAnalysisRequestsInput は解析依頼一覧ユースケースの入力。UserID は認証済みの利用者、YearMonth はパスパラメータの YYYY-MM。
type ListAnalysisRequestsInput struct {
	UserID    string
	YearMonth string
}

// ListAnalysisRequestsOutput はその月の解析依頼。作成日時の降順で、該当が無ければ空のスライス。
type ListAnalysisRequestsOutput struct {
	Requests []domain.AnalysisRequest
}

// ListAnalysisRequestsUsecase は利用者の指定月の解析依頼を返す。
// 画面は数秒おきにこれを呼んで status の変化(ANALYZING → SUCCEEDED / FAILED)を追う。
type ListAnalysisRequestsUsecase struct {
	Requests AnalysisRequestLister
}

// ListAnalysisRequestsUsecaseInterface は HTTP 層が解析依頼一覧ユースケースへ依存するための契約。
type ListAnalysisRequestsUsecaseInterface interface {
	List(ctx context.Context, in ListAnalysisRequestsInput) (ListAnalysisRequestsOutput, error)
}

var _ ListAnalysisRequestsUsecaseInterface = (*ListAnalysisRequestsUsecase)(nil)

// NewListAnalysisRequestsUsecase は解析依頼一覧ユースケースを生成する。
func NewListAnalysisRequestsUsecase(requests AnalysisRequestLister) ListAnalysisRequestsUsecaseInterface {
	return &ListAnalysisRequestsUsecase{Requests: requests}
}

// List は入力を検証してから解析依頼を引く。利用者が空、または月が YYYY-MM でなければ ErrInvalidInput。
// 一覧は月をそのまま GSI のキーに使うので、形式が違えば空の結果になる前に入力の誤りとして弾く。
func (u *ListAnalysisRequestsUsecase) List(ctx context.Context, in ListAnalysisRequestsInput) (ListAnalysisRequestsOutput, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return ListAnalysisRequestsOutput{}, ErrInvalidInput
	}
	month, err := common.NewYearMonth(in.YearMonth)
	if err != nil {
		return ListAnalysisRequestsOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	// ここから先のログ(dynamodb_query span)に user_id / year_month が付く
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.String("year_month", month.String()))
	requests, err := u.Requests.ListByMonth(ctx, in.UserID, month.String())
	if err != nil {
		return ListAnalysisRequestsOutput{}, fmt.Errorf("list analysis requests: %w", err)
	}
	return ListAnalysisRequestsOutput{Requests: requests}, nil
}
