// Package application はレシート解析のユースケースを提供する。
package application

import (
	"context"
	"errors"
	"fmt"

	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

// SpanAnalysis はジョブ 1 件の処理区間の名前。
// 結果(analysis_status / error_code / 件数 / トークン数)は span_finished の 1 行にまとまるので、
// 成功率や所要時間はこの行だけで集計できる。
const SpanAnalysis = "analysis"

// ログの event 値。命名は "<対象>_<過去形の動詞>"。Logs Insights の filter はこの定数の値で書く。
const (
	EventImageDecodeFailed           = "image_decode_failed"
	EventOpenAIRequestFailed         = "openai_request_failed"
	EventOpenAIResponseRejected      = "openai_response_rejected"
	EventAnalysisValidationFailed    = "analysis_validation_failed"
	EventBillingRegistrationRejected = "billing_registration_rejected"
)

// AnalyzeReceiptInput はレシート解析ユースケースの入力。
type AnalyzeReceiptInput struct {
	Job domain.Job
}

// AnalyzeReceiptOutput はレシート解析ユースケースの出力。
type AnalyzeReceiptOutput struct {
	Status domain.Status
	// ErrorCode は Status が FAILED のときの失敗コード。
	ErrorCode string
	// BillingID は Status が SUCCEEDED のとき登録した請求の ID。
	BillingID string
	// RawResultKey は OpenAI の生レスポンスの保存先。保存しなかった場合は空。
	RawResultKey string
}

// AnalyzeReceiptUsecase は SQS から受け取ったジョブ 1 件を解析し、結果を upload_histories と請求テーブルに反映する。
type AnalyzeReceiptUsecase struct {
	Images     ReceiptImageReader
	Resizer    ImageResizer
	Analyzer   ReceiptAnalyzer
	RawResults RawResultStore
	Histories  UploadHistoryRepository
	Billings   BillingRegistrar
	IDs        IDGenerator
	Clock      timewrapper.Interface
	Log        logger.Interface
}

// AnalyzeReceiptUsecaseInterface は Lambda 層がレシート解析ユースケースへ依存するための契約。
type AnalyzeReceiptUsecaseInterface interface {
	Analyze(ctx context.Context, in AnalyzeReceiptInput) (AnalyzeReceiptOutput, error)
}

var _ AnalyzeReceiptUsecaseInterface = (*AnalyzeReceiptUsecase)(nil)

// NewAnalyzeReceiptUsecase はレシート解析ユースケースを生成する。
func NewAnalyzeReceiptUsecase(images ReceiptImageReader, resizer ImageResizer, analyzer ReceiptAnalyzer, rawResults RawResultStore, histories UploadHistoryRepository, billings BillingRegistrar, ids IDGenerator, clock timewrapper.Interface, log logger.Interface) AnalyzeReceiptUsecaseInterface {
	return &AnalyzeReceiptUsecase{
		Images:     images,
		Resizer:    resizer,
		Analyzer:   analyzer,
		RawResults: rawResults,
		Histories:  histories,
		Billings:   billings,
		IDs:        ids,
		Clock:      clock,
		Log:        log,
	}
}

// Analyze はジョブ 1 件を analysis span として処理する。
//
// 戻り値の error は一時的または予期しない失敗(S3 / OpenAI / DynamoDB の障害など)で、呼び出し側は
// Lambda を失敗させてキューの再配信に任せる。業務上の失敗(読めない画像、不正なレシート)は error にせず、
// upload_histories に FAILED として記録して Status で返す。
func (u *AnalyzeReceiptUsecase) Analyze(ctx context.Context, in AnalyzeReceiptInput) (AnalyzeReceiptOutput, error) {
	job := in.Job
	// ここから先の全ログに upload_id などが付く。下層はログに毎回書かなくてよい
	ctx = logger.ContextWith(ctx,
		logger.UserID(job.UserID),
		logger.UploadID(job.UploadID),
		logger.Int("attempt", job.Attempt),
		logger.String("trigger", string(job.Trigger)),
	)
	ctx, span := logger.StartSpan(ctx, u.Log, SpanAnalysis, logger.String("s3_key", job.Key))
	out, err := u.analyze(ctx, span, job)
	span.End(err)
	return out, err
}

func (u *AnalyzeReceiptUsecase) analyze(ctx context.Context, span *logger.Span, job domain.Job) (AnalyzeReceiptOutput, error) {
	started, err := u.Histories.MarkAnalyzing(ctx, job, u.Clock.Now())
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	if !started {
		span.AddFields(logger.String("analysis_status", string(domain.StatusSkipped)))
		return AnalyzeReceiptOutput{Status: domain.StatusSkipped}, nil
	}

	data, err := u.Images.ReadImage(ctx, job.Bucket, job.Key)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.Int("image_bytes", len(data)))
	jpeg, err := u.Resizer.ResizeJPEG(data)
	if err != nil {
		u.Log.Error(ctx, "failed to decode image", logger.Event(EventImageDecodeFailed), logger.Err(err))
		return u.fail(ctx, span, job, domain.Failure{Code: domain.FailureInternal, Message: "画像を読み込めませんでした"}, "")
	}

	result, err := u.Analyzer.Analyze(ctx, jpeg)
	if err != nil {
		u.Log.Error(ctx, "OpenAI request failed", logger.Event(EventOpenAIRequestFailed), logger.Err(err))
		return AnalyzeReceiptOutput{}, err
	}
	responseID, rawKey, err := u.saveRawResult(ctx, job, result)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	// 以降のログと span_finished には OpenAI のレスポンス ID と生結果の保存先を必ず付ける
	ids := []logger.Field{logger.String("response_id", responseID), logger.String("raw_result_s3_key", rawKey)}
	ctx = logger.ContextWith(ctx, ids...)
	span.AddFields(ids...)
	span.AddFields(
		logger.Int("input_tokens", result.Usage.InputTokens),
		logger.Int("reasoning_tokens", result.Usage.ReasoningTokens),
		logger.Int("output_tokens", result.Usage.OutputTokens),
	)
	if result.Failure != nil {
		u.Log.Error(ctx, "OpenAI response rejected", logger.Event(EventOpenAIResponseRejected), logger.String("error_code", result.Failure.Code))
		return u.fail(ctx, span, job, *result.Failure, rawKey)
	}

	receipt, failure := domain.Validate(result.Receipt, u.Clock.Now())
	span.AddFields(logger.Int("detail_count", len(receipt.Details)))
	if failure != nil {
		u.Log.Error(ctx, "analysis validation failed", logger.Event(EventAnalysisValidationFailed), logger.String("error_code", failure.Code))
		return u.fail(ctx, span, job, *failure, rawKey)
	}
	if len(receipt.Details) == 0 {
		span.AddFields(logger.String("analysis_status", string(domain.StatusNoData)))
		if err := u.Histories.MarkNoData(ctx, job, rawKey, u.Clock.Now()); err != nil {
			return AnalyzeReceiptOutput{}, err
		}
		return AnalyzeReceiptOutput{Status: domain.StatusNoData, RawResultKey: rawKey}, nil
	}

	billing, err := u.newBilling(job, receipt)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.BillingID(billing.ID))
	err = u.Billings.Register(ctx, job, billing, rawKey, u.Clock.Now())
	if errors.Is(err, ErrBillingRejected) {
		u.Log.Error(ctx, "billing registration rejected", logger.Event(EventBillingRegistrationRejected), logger.Err(err))
		return u.fail(ctx, span, job, domain.Failure{Code: domain.FailureInternal, Message: "解析結果を登録できませんでした"}, rawKey)
	}
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.String("analysis_status", string(domain.StatusSucceeded)))
	return AnalyzeReceiptOutput{Status: domain.StatusSucceeded, BillingID: billing.ID, RawResultKey: rawKey}, nil
}

// saveRawResult は生レスポンスを保存し、レスポンス ID と保存先のキーを返す。
// レスポンス ID がなければ採番し、本文がなければ保存せず空のキーを返す。
func (u *AnalyzeReceiptUsecase) saveRawResult(ctx context.Context, job domain.Job, result AnalysisResult) (responseID, rawKey string, err error) {
	responseID = result.ResponseID
	if responseID == "" {
		responseID, err = u.IDs.NewID()
		if err != nil {
			return "", "", fmt.Errorf("generate response id: %w", err)
		}
	}
	if len(result.Raw) == 0 {
		return responseID, "", nil
	}
	rawKey, err = u.RawResults.SaveRawResult(ctx, job, responseID, result.Raw)
	if err != nil {
		return "", "", err
	}
	return responseID, rawKey, nil
}

func (u *AnalyzeReceiptUsecase) fail(ctx context.Context, span *logger.Span, job domain.Job, failure domain.Failure, rawKey string) (AnalyzeReceiptOutput, error) {
	span.AddFields(logger.String("analysis_status", string(domain.StatusFailed)), logger.String("error_code", failure.Code))
	if err := u.Histories.MarkFailed(ctx, job, failure, rawKey, u.Clock.Now()); err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	return AnalyzeReceiptOutput{Status: domain.StatusFailed, ErrorCode: failure.Code, RawResultKey: rawKey}, nil
}

// newBilling は検証済みのレシートから請求を組み立て、請求と明細の ID を採番する。
func (u *AnalyzeReceiptUsecase) newBilling(job domain.Job, receipt domain.Receipt) (domain.Billing, error) {
	billingID, err := u.IDs.NewID()
	if err != nil {
		return domain.Billing{}, fmt.Errorf("generate billing id: %w", err)
	}
	billing := domain.Billing{
		ID:          billingID,
		UserID:      job.UserID,
		UploadID:    job.UploadID,
		PurchasedAt: *receipt.PurchasedAt,
		TotalAmount: *receipt.TotalAmount,
		CreatedAt:   u.Clock.Now(),
		Details:     make([]domain.BillingDetail, 0, len(receipt.Details)),
	}
	if receipt.StoreName != nil {
		billing.StoreName = *receipt.StoreName
	}
	for _, d := range receipt.Details {
		id, err := u.IDs.NewID()
		if err != nil {
			return domain.Billing{}, fmt.Errorf("generate detail id: %w", err)
		}
		billing.Details = append(billing.Details, domain.BillingDetail{ID: id, Name: d.Name, Category: d.Category, Amount: d.Amount, Quantity: d.Quantity})
	}
	return billing, nil
}
