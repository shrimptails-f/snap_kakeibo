// Package application はレシート解析のユースケースを提供する。
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	ledgerdomain "snap_kakeibo/backend/internal/ledger/domain"
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
	EventExpenseRegistrationRejected = "expense_registration_rejected"
)

// AnalyzeReceiptInput はレシート解析ユースケースの入力。
type AnalyzeReceiptInput struct {
	Job domain.AnalysisJob
}

// AnalyzeReceiptOutput はレシート解析ユースケースの出力。
type AnalyzeReceiptOutput struct {
	Outcome domain.AnalysisOutcome
	// ErrorCode は Outcome が FAILED のときの失敗コード。
	ErrorCode string
	// ExpenseID は Outcome が SUCCEEDED のとき登録した支出の ID。
	ExpenseID string
	// RawResultKey は OpenAI の生レスポンスの保存先。保存しなかった場合は空。
	RawResultKey string
}

// AnalyzeReceiptUsecase は SQS から受け取ったジョブ 1 件を解析し、結果を analysis_requests と家計簿のテーブルに反映する。
type AnalyzeReceiptUsecase struct {
	Images     ReceiptImageReader
	Resizer    ImageResizer
	Analyzer   ReceiptAnalyzer
	RawResults RawResultStore
	Requests   AnalysisRequestRepository
	Expenses   ExpenseRegistrar
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
func NewAnalyzeReceiptUsecase(images ReceiptImageReader, resizer ImageResizer, analyzer ReceiptAnalyzer, rawResults RawResultStore, requests AnalysisRequestRepository, expenses ExpenseRegistrar, ids IDGenerator, clock timewrapper.Interface, log logger.Interface) AnalyzeReceiptUsecaseInterface {
	return &AnalyzeReceiptUsecase{
		Images:     images,
		Resizer:    resizer,
		Analyzer:   analyzer,
		RawResults: rawResults,
		Requests:   requests,
		Expenses:   expenses,
		IDs:        ids,
		Clock:      clock,
		Log:        log,
	}
}

// Analyze はジョブ 1 件を analysis span として処理する。
//
// 戻り値の error は一時的または予期しない失敗(S3 / OpenAI / DynamoDB の障害など)で、呼び出し側は
// Lambda を失敗させてキューの再配信に任せる。業務上の失敗(読めない画像、不正なレシート)は error にせず、
// analysis_requests に FAILED として記録して Outcome で返す。
func (u *AnalyzeReceiptUsecase) Analyze(ctx context.Context, in AnalyzeReceiptInput) (AnalyzeReceiptOutput, error) {
	job := in.Job
	// ここから先の全ログに analysis_request_id などが付く。下層はログに毎回書かなくてよい
	ctx = logger.ContextWith(ctx,
		logger.UserID(job.UserID),
		logger.AnalysisRequestID(job.AnalysisRequestID),
		logger.Int("attempt", job.Attempt),
		logger.String("trigger", string(job.Trigger)),
	)
	ctx, span := logger.StartSpan(ctx, u.Log, SpanAnalysis, logger.String("s3_key", job.Key))
	out, err := u.analyze(ctx, span, job)
	span.End(err)
	return out, err
}

func (u *AnalyzeReceiptUsecase) analyze(ctx context.Context, span *logger.Span, job domain.AnalysisJob) (AnalyzeReceiptOutput, error) {
	started, err := u.Requests.MarkAnalyzing(ctx, job, u.Clock.Now())
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	if !started {
		span.AddFields(logger.String("analysis_status", string(domain.OutcomeSkipped)))
		return AnalyzeReceiptOutput{Outcome: domain.OutcomeSkipped}, nil
	}

	data, err := u.Images.ReadImage(ctx, job.Bucket, job.Key)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.Int("image_bytes", len(data)))
	jpeg, err := u.Resizer.ResizeJPEG(data)
	if err != nil {
		u.Log.Error(ctx, "failed to decode image", logger.Event(EventImageDecodeFailed), logger.Err(err))
		return u.fail(ctx, span, job, domain.InternalFailure("画像を読み込めませんでした"), "")
	}

	response, err := u.Analyzer.Analyze(ctx, jpeg)
	if err != nil {
		u.Log.Error(ctx, "OpenAI request failed", logger.Event(EventOpenAIRequestFailed), logger.Err(err))
		return AnalyzeReceiptOutput{}, err
	}
	responseID, rawKey, err := u.saveRawResult(ctx, job, response)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	// 以降のログと span_finished には OpenAI のレスポンス ID と生結果の保存先を必ず付ける
	ids := []logger.Field{logger.String("response_id", responseID), logger.String("raw_result_s3_key", rawKey)}
	ctx = logger.ContextWith(ctx, ids...)
	span.AddFields(ids...)
	span.AddFields(
		logger.Int("input_tokens", response.Usage.InputTokens),
		logger.Int("reasoning_tokens", response.Usage.ReasoningTokens),
		logger.Int("output_tokens", response.Usage.OutputTokens),
	)
	if response.Failure != nil {
		u.Log.Error(ctx, "OpenAI response rejected", logger.Event(EventOpenAIResponseRejected), logger.String("error_code", response.Failure.Code()))
		return u.fail(ctx, span, job, *response.Failure, rawKey)
	}

	result, failure := domain.ValidateReading(response.Reading, u.Clock.Now())
	if failure != nil {
		u.Log.Error(ctx, "analysis validation failed", logger.Event(EventAnalysisValidationFailed), logger.String("error_code", failure.Code()))
		return u.fail(ctx, span, job, *failure, rawKey)
	}
	span.AddFields(logger.Int("detail_count", len(result.Details())))
	if !result.HasData() {
		span.AddFields(logger.String("analysis_status", string(domain.OutcomeNoData)))
		if err := u.Requests.MarkNoData(ctx, job, rawKey, u.Clock.Now()); err != nil {
			return AnalyzeReceiptOutput{}, err
		}
		return AnalyzeReceiptOutput{Outcome: domain.OutcomeNoData, RawResultKey: rawKey}, nil
	}

	expense, err := u.newExpense(job, result)
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.ExpenseID(expense.ID().String()))
	err = u.Expenses.Register(ctx, job, expense, rawKey, u.Clock.Now())
	if errors.Is(err, ErrExpenseRejected) {
		u.Log.Error(ctx, "expense registration rejected", logger.Event(EventExpenseRegistrationRejected), logger.Err(err))
		return u.fail(ctx, span, job, domain.InternalFailure("解析結果を登録できませんでした"), rawKey)
	}
	if err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	span.AddFields(logger.String("analysis_status", string(domain.OutcomeSucceeded)))
	return AnalyzeReceiptOutput{Outcome: domain.OutcomeSucceeded, ExpenseID: expense.ID().String(), RawResultKey: rawKey}, nil
}

// saveRawResult は生レスポンスを保存し、レスポンス ID と保存先のキーを返す。
// レスポンス ID がなければ採番し、本文がなければ保存せず空のキーを返す。
func (u *AnalyzeReceiptUsecase) saveRawResult(ctx context.Context, job domain.AnalysisJob, response AnalyzerResponse) (responseID, rawKey string, err error) {
	responseID = response.ResponseID
	if responseID == "" {
		responseID, err = u.IDs.NewID()
		if err != nil {
			return "", "", fmt.Errorf("generate response id: %w", err)
		}
	}
	if len(response.Raw) == 0 {
		return responseID, "", nil
	}
	rawKey, err = u.RawResults.SaveRawResult(ctx, job, responseID, response.Raw)
	if err != nil {
		return "", "", err
	}
	return responseID, rawKey, nil
}

func (u *AnalyzeReceiptUsecase) fail(ctx context.Context, span *logger.Span, job domain.AnalysisJob, reason domain.FailureReason, rawKey string) (AnalyzeReceiptOutput, error) {
	span.AddFields(logger.String("analysis_status", string(domain.OutcomeFailed)), logger.String("error_code", reason.Code()))
	if err := u.Requests.MarkFailed(ctx, job, reason, rawKey, u.Clock.Now()); err != nil {
		return AnalyzeReceiptOutput{}, err
	}
	return AnalyzeReceiptOutput{Outcome: domain.OutcomeFailed, ErrorCode: reason.Code(), RawResultKey: rawKey}, nil
}

// newExpense は検証済みの解析結果を家計簿コンテキストの支出集約へ変換し、支出と支出明細の ID を採番する。
// 解析側の型を家計簿側へ持ち込まず、ここで境界を越える。
func (u *AnalyzeReceiptUsecase) newExpense(job domain.AnalysisJob, result domain.AnalysisResult) (ledgerdomain.Expense, error) {
	id, err := u.IDs.NewID()
	if err != nil {
		return ledgerdomain.Expense{}, fmt.Errorf("generate expense id: %w", err)
	}
	expenseID, _ := common.NewExpenseID(id)
	userID, _ := common.NewUserID(job.UserID)
	requestID, _ := common.NewAnalysisRequestID(job.AnalysisRequestID)
	analyzed := result.Details()
	details := make([]ledgerdomain.ExpenseDetail, 0, len(analyzed))
	for _, d := range analyzed {
		id, err := u.IDs.NewID()
		if err != nil {
			return ledgerdomain.Expense{}, fmt.Errorf("generate detail id: %w", err)
		}
		detailID, _ := ledgerdomain.NewExpenseDetailID(id)
		detail, err := ledgerdomain.NewExpenseDetail(detailID, d.Name(), d.Amount(), d.Quantity(), d.Category(), ledgerdomain.CategorySourceAI)
		if err != nil {
			return ledgerdomain.Expense{}, fmt.Errorf("build expense detail: %w", err)
		}
		details = append(details, detail)
	}
	expense, err := ledgerdomain.NewExpense(expenseID, userID, requestID, result.StoreName(), result.PurchaseDate(), result.ReadAmount(), details)
	if err != nil {
		return ledgerdomain.Expense{}, fmt.Errorf("build expense: %w", err)
	}
	evidence, err := json.Marshal(result.Evidence())
	if err != nil {
		return ledgerdomain.Expense{}, fmt.Errorf("marshal analysis evidence: %w", err)
	}
	expense.SetAnalysisEvidence(string(evidence))
	return expense, nil
}
