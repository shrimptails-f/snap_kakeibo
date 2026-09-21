package application

import (
	"context"
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/upload/domain"
)

// ListUploadsInput は一覧ユースケースの入力。UserID は認証済みの利用者、YearMonth はパスパラメータの YYYY-MM。
type ListUploadsInput struct {
	UserID    string
	YearMonth string
}

// ListUploadsOutput はその月の履歴。作成日時の降順で、該当が無ければ空のスライス。
type ListUploadsOutput struct {
	Uploads []domain.UploadHistory
}

// ListUploadsUsecase は利用者の指定月のアップロード履歴を返す。
// 画面は数秒おきにこれを呼んで status の変化(ANALYZING → SUCCEEDED / FAILED)を追う。
type ListUploadsUsecase struct {
	Histories UploadHistoryLister
}

// ListUploadsUsecaseInterface は HTTP 層が一覧ユースケースへ依存するための契約。
type ListUploadsUsecaseInterface interface {
	List(ctx context.Context, in ListUploadsInput) (ListUploadsOutput, error)
}

var _ ListUploadsUsecaseInterface = (*ListUploadsUsecase)(nil)

// NewListUploadsUsecase は一覧ユースケースを生成する。
func NewListUploadsUsecase(histories UploadHistoryLister) ListUploadsUsecaseInterface {
	return &ListUploadsUsecase{Histories: histories}
}

// List は入力を検証してから履歴を引く。利用者が空、または月が YYYY-MM でなければ ErrInvalidInput。
func (u *ListUploadsUsecase) List(ctx context.Context, in ListUploadsInput) (ListUploadsOutput, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return ListUploadsOutput{}, ErrInvalidInput
	}
	if err := domain.ValidateYearMonth(in.YearMonth); err != nil {
		return ListUploadsOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	// ここから先のログ(dynamodb_query span)に user_id / year_month が付く
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.String("year_month", in.YearMonth))
	uploads, err := u.Histories.ListByMonth(ctx, in.UserID, in.YearMonth)
	if err != nil {
		return ListUploadsOutput{}, fmt.Errorf("list upload histories: %w", err)
	}
	return ListUploadsOutput{Uploads: uploads}, nil
}
