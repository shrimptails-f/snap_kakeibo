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

// DefaultUploadURLTTL は署名付き URL と analysis_requests.upload_expires_at の有効期間。
const DefaultUploadURLTTL = 15 * time.Minute

// CreateUploadInput はアップロード開始ユースケースの入力。UserID は認証済みの利用者。
// FileName / ContentType は空なら domain の既定値を使う。
type CreateUploadInput struct {
	UserID      string
	FileName    string
	ContentType string
}

// CreateUploadOutput はクライアントが POST に使う情報。
type CreateUploadOutput struct {
	AnalysisRequestID string
	S3Key             string
	PostForm          UploadForm
	ExpiresAt         time.Time
}

// CreateUploadUsecase は analysis_request_id を採番し、UPLOADING の解析依頼を登録してから署名済み POST フォームを返す。
// 解析依頼を先に登録するのは、POST 完了の S3 通知を受けた analyze-receipt が解析依頼を前提に状態遷移するため。
type CreateUploadUsecase struct {
	Requests AnalysisRequestRepository
	URLs     UploadFormPresigner
	IDs      IDGenerator
	Clock    timewrapper.Interface
	// URLTTL は署名付き URL の有効期間。0 なら DefaultUploadURLTTL
	URLTTL time.Duration
}

// CreateUploadUsecaseInterface は HTTP 層がアップロード開始ユースケースへ依存するための契約。
type CreateUploadUsecaseInterface interface {
	Create(ctx context.Context, in CreateUploadInput) (CreateUploadOutput, error)
}

var _ CreateUploadUsecaseInterface = (*CreateUploadUsecase)(nil)

// NewCreateUploadUsecase はアップロード開始ユースケースを生成する。
func NewCreateUploadUsecase(requests AnalysisRequestRepository, urls UploadFormPresigner, ids IDGenerator, clock timewrapper.Interface) CreateUploadUsecaseInterface {
	return &CreateUploadUsecase{Requests: requests, URLs: urls, IDs: ids, Clock: clock, URLTTL: DefaultUploadURLTTL}
}

// Create は解析依頼を登録し、署名済み POST フォームを返す。
// 解析依頼の登録に失敗した場合は URL を発行しない。
func (u *CreateUploadUsecase) Create(ctx context.Context, in CreateUploadInput) (CreateUploadOutput, error) {
	requestID, err := u.IDs.NewID()
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("generate analysis request id: %w", err)
	}
	ttl := u.URLTTL
	if ttl <= 0 {
		ttl = DefaultUploadURLTTL
	}
	request, err := domain.NewUploadRequest(in.UserID, requestID, in.FileName, in.ContentType, u.Clock.Now(), ttl)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidUpload) {
			return CreateUploadOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		return CreateUploadOutput{}, err
	}
	// ここから先のログ(dynamodb_put_item span など)に analysis_request_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(request.UserID().String()), logger.AnalysisRequestID(request.ID().String()))
	if err := u.Requests.Save(ctx, request); err != nil {
		return CreateUploadOutput{}, err
	}
	image := request.Image()
	postForm, err := u.URLs.PresignPost(ctx, image.Reference(), image.ContentType(), ttl)
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("presign upload url: %w", err)
	}
	return CreateUploadOutput{AnalysisRequestID: request.ID().String(), S3Key: image.Reference(), PostForm: postForm, ExpiresAt: request.UploadExpiresAt()}, nil
}
