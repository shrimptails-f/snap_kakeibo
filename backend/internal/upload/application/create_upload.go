// Package application はレシート画像アップロードのユースケースを提供する。
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/domain"
)

// DefaultUploadURLTTL は署名付き URL と upload_histories.expires_at の有効期間。
const DefaultUploadURLTTL = 15 * time.Minute

// CreateUploadInput はアップロード開始ユースケースの入力。UserID は認証済みの利用者。
// FileName / ContentType は空なら domain の既定値を使う。
type CreateUploadInput struct {
	UserID      string
	FileName    string
	ContentType string
}

// CreateUploadOutput はクライアントが PUT に使う情報。
type CreateUploadOutput struct {
	UploadID  string
	S3Key     string
	PutURL    string
	ExpiresAt time.Time
}

// CreateUploadUsecase は upload_id を採番し、UPLOADING の履歴を登録してから署名付き PUT URL を返す。
// 履歴を先に登録するのは、PUT 完了の S3 通知を受けた analyze-receipt が履歴を前提に状態遷移するため。
type CreateUploadUsecase struct {
	Histories UploadHistoryRepository
	URLs      UploadURLPresigner
	IDs       IDGenerator
	Clock     timewrapper.Interface
	// URLTTL は署名付き URL の有効期間。0 なら DefaultUploadURLTTL
	URLTTL time.Duration
}

// CreateUploadUsecaseInterface は HTTP 層がアップロード開始ユースケースへ依存するための契約。
type CreateUploadUsecaseInterface interface {
	Create(ctx context.Context, in CreateUploadInput) (CreateUploadOutput, error)
}

var _ CreateUploadUsecaseInterface = (*CreateUploadUsecase)(nil)

// NewCreateUploadUsecase はアップロード開始ユースケースを生成する。
func NewCreateUploadUsecase(histories UploadHistoryRepository, urls UploadURLPresigner, ids IDGenerator, clock timewrapper.Interface) CreateUploadUsecaseInterface {
	return &CreateUploadUsecase{Histories: histories, URLs: urls, IDs: ids, Clock: clock, URLTTL: DefaultUploadURLTTL}
}

// Create は履歴を登録し、署名付き PUT URL を返す。
// 履歴の登録に失敗した場合は URL を発行しない。
func (u *CreateUploadUsecase) Create(ctx context.Context, in CreateUploadInput) (CreateUploadOutput, error) {
	uploadID, err := u.IDs.NewID()
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("generate upload id: %w", err)
	}
	ttl := u.URLTTL
	if ttl <= 0 {
		ttl = DefaultUploadURLTTL
	}
	history, err := domain.NewUploadHistory(in.UserID, uploadID, in.FileName, in.ContentType, u.Clock.Now(), ttl)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidUploadHistory) {
			return CreateUploadOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		return CreateUploadOutput{}, err
	}
	// ここから先のログ(dynamodb_put_item span など)に upload_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(history.UserID), logger.UploadID(history.UploadID))
	if err := u.Histories.Save(ctx, history); err != nil {
		return CreateUploadOutput{}, err
	}
	putURL, err := u.URLs.PresignPut(ctx, history.S3Key, history.ContentType, ttl)
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("presign upload url: %w", err)
	}
	return CreateUploadOutput{UploadID: history.UploadID, S3Key: history.S3Key, PutURL: putURL, ExpiresAt: history.ExpiresAt}, nil
}
