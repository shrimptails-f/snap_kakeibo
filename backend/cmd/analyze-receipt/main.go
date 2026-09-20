package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"snap_kakeibo/backend/internal/analyze"
	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	libsqs "snap_kakeibo/backend/internal/library/sqs"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
)

type job struct {
	UserID   string `json:"user_id"`
	UploadID string `json:"upload_id"`
	Attempt  int    `json:"attempt"`
	Trigger  string `json:"trigger"`
	Bucket   string `json:"-"`
	Key      string `json:"-"`
}
type billing struct {
	PK             string `dynamodbav:"PK"`
	SK             string `dynamodbav:"SK"`
	Type           string `dynamodbav:"type"`
	BillingID      string `dynamodbav:"billing_id"`
	UploadID       string `dynamodbav:"upload_id"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	Source         string `dynamodbav:"source"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
	OriginalAmount int64  `dynamodbav:"original_amount"`
	DiscountAmount int64  `dynamodbav:"discount_amount"`
	FinalAmount    int64  `dynamodbav:"final_amount"`
	IsEdited       bool   `dynamodbav:"is_edited"`
}
type detail struct {
	PK             string `dynamodbav:"PK"`
	SK             string `dynamodbav:"SK"`
	GSI1PK         string `dynamodbav:"GSI1PK"`
	GSI1SK         string `dynamodbav:"GSI1SK"`
	Type           string `dynamodbav:"type"`
	DetailID       string `dynamodbav:"detail_id"`
	BillingID      string `dynamodbav:"billing_id"`
	UploadID       string `dynamodbav:"upload_id"`
	Name           string `dynamodbav:"name"`
	Category       string `dynamodbav:"category"`
	CategorySource string `dynamodbav:"category_source"`
	Source         string `dynamodbav:"source"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
	IsEdited       bool   `dynamodbav:"is_edited"`
}

// ログの event 値。命名は "<対象>_<過去形の動詞>"。Logs Insights の filter はこの定数の値で書く。
const (
	eventQueueMessageInvalid      = "queue_message_invalid"
	eventImageDecodeFailed        = "image_decode_failed"
	eventOpenAIRequestFailed      = "openai_request_failed"
	eventOpenAIResponseRejected   = "openai_response_rejected"
	eventAnalysisValidationFailed = "analysis_validation_failed"
)

// analysis span の結果。analysis_status に載せる。upload_histories の status と同じ語彙を使う。
const (
	analysisSucceeded = "SUCCEEDED"
	analysisFailed    = "FAILED"
	analysisNoData    = "NO_DATA"
	analysisSkipped   = "SKIPPED"
)

var (
	cfg          = app.LoadConfig()
	log          = logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	ddb          *dynamodb.Client
	s3c          *s3.Client
	ssmc         *ssm.Client
	settingsOnce sync.Once
	settings     analyze.Client
	settingsErr  error
)

func init() {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	s3c = s3.NewFromConfig(awsCfg)
	ssmc = ssm.NewFromConfig(awsCfg)
}

func handler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		// レコードが運んできた trace を引き継ぎ、message_id を積む
		rctx := libsqs.RecordContext(ctx, record)
		jobs, err := decodeJobs(record.Body)
		if err != nil {
			log.Error(rctx, "failed to decode queue message", logger.Event(eventQueueMessageInvalid), logger.Err(err))
			return err
		}
		for _, j := range jobs {
			// ここから先の全ログに upload_id などが付く。下層の関数はログに毎回書かなくてよい
			jctx := logger.ContextWith(rctx,
				logger.UserID(j.UserID),
				logger.UploadID(j.UploadID),
				logger.Int("attempt", j.Attempt),
				logger.String("trigger", j.Trigger),
			)
			if err := process(jctx, j); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeJobs(body string) ([]job, error) {
	var retry job
	if json.Unmarshal([]byte(body), &retry) == nil && retry.Trigger == "RETRY" && retry.UserID != "" && retry.UploadID != "" && retry.Attempt > 0 {
		retry.Bucket = cfg.ReceiptBucket
		retry.Key = "receipts/" + retry.UserID + "/" + retry.UploadID + "/original.jpg"
		return []job{retry}, nil
	}
	var event events.S3Event
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return nil, fmt.Errorf("decode queue message: %w", err)
	}
	jobs := make([]job, 0, len(event.Records))
	for _, r := range event.Records {
		key, err := url.QueryUnescape(r.S3.Object.Key)
		if err != nil {
			return nil, err
		}
		userID, uploadID, ok := idsFromKey(key)
		if ok {
			jobs = append(jobs, job{UserID: userID, UploadID: uploadID, Attempt: 1, Trigger: "S3", Bucket: r.S3.Bucket.Name, Key: key})
		}
	}
	return jobs, nil
}

func idsFromKey(key string) (string, string, bool) {
	p := strings.Split(key, "/")
	if len(p) != 4 || p[0] != "receipts" || p[1] == "" || p[2] == "" || p[3] == "" {
		return "", "", false
	}
	return p[1], p[2], true
}

// process はジョブ 1 件を "analysis" span として処理する。
// 結果(analysis_status / error_code / 件数 / トークン数)は span_finished の 1 行にまとまるので、
// 成功率や所要時間はこの行だけで集計できる。
func process(ctx context.Context, j job) error {
	ctx, span := logger.StartSpan(ctx, log, "analysis", logger.String("s3_key", j.Key))
	err := analyzeJob(ctx, span, j)
	span.End(err)
	return err
}

func analyzeJob(ctx context.Context, span *logger.Span, j job) error {
	started, err := markAnalyzing(ctx, j)
	if err != nil {
		return err
	}
	if !started {
		span.AddFields(logger.String("analysis_status", analysisSkipped))
		return nil
	}
	out, err := s3c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(j.Bucket), Key: aws.String(j.Key)})
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(out.Body, 30<<20))
	_ = out.Body.Close()
	if err != nil {
		return err
	}
	span.AddFields(logger.Int("image_bytes", len(data)))
	edge := 2048
	if v, e := strconv.Atoi(cfg.ImageMaxEdge); e == nil && v > 0 {
		edge = v
	}
	jpegData, err := analyze.ResizeJPEG(data, edge)
	if err != nil {
		log.Error(ctx, "failed to decode image", logger.Event(eventImageDecodeFailed), logger.Err(err))
		return markFailed(ctx, span, j, "INTERNAL", "画像を読み込めませんでした", "")
	}
	client, err := openAISettings(ctx)
	if err != nil {
		return err
	}
	raw, callErr := callOpenAI(ctx, client, jpegData)
	if errors.Is(callErr, analyze.ErrTemporary) {
		log.Error(ctx, "OpenAI request failed temporarily", logger.Event(eventOpenAIRequestFailed), logger.Bool("temporary", true), logger.Err(callErr))
		return callErr
	}
	responseID := responseID(raw)
	if responseID == "" {
		responseID, err = app.NewID()
		if err != nil {
			return err
		}
	}
	rawKey := fmt.Sprintf("analysis-results/%s/%s/%d/%s.json", j.UserID, j.UploadID, j.Attempt, safeID(responseID))
	if len(raw) > 0 {
		if _, err := s3c.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(cfg.ReceiptBucket), Key: aws.String(rawKey), Body: bytes.NewReader(raw), ContentType: aws.String("application/json")}); err != nil {
			return err
		}
	} else {
		rawKey = ""
	}
	// 以降のログと span_finished には OpenAI のレスポンス ID と生結果の保存先を必ず付ける
	ids := []logger.Field{logger.String("response_id", responseID), logger.String("raw_result_s3_key", rawKey)}
	ctx = logger.ContextWith(ctx, ids...)
	span.AddFields(ids...)
	if callErr != nil {
		var f *analyze.Failure
		if errors.As(callErr, &f) {
			logOpenAIFailure(ctx, f)
			return markFailed(ctx, span, j, f.Code, f.Message, rawKey)
		}
		log.Error(ctx, "OpenAI request failed", logger.Event(eventOpenAIRequestFailed), logger.Err(callErr))
		return callErr
	}
	receipt, resp, err := analyze.ParseResponse(raw)
	span.AddFields(
		logger.Int("input_tokens", resp.Usage.InputTokens),
		logger.Int("reasoning_tokens", resp.Usage.OutputTokensDetails.ReasoningTokens),
		logger.Int("output_tokens", resp.Usage.OutputTokens),
	)
	if err != nil {
		var f *analyze.Failure
		if errors.As(err, &f) {
			log.Error(ctx, "OpenAI response rejected", logger.Event(eventOpenAIResponseRejected), logger.String("error_code", f.Code))
			return markFailed(ctx, span, j, f.Code, f.Message, rawKey)
		}
		return err
	}
	receipt, failure := analyze.Validate(receipt, time.Now())
	span.AddFields(logger.Int("detail_count", len(receipt.Details)))
	if failure != nil {
		log.Error(ctx, "analysis validation failed", logger.Event(eventAnalysisValidationFailed), logger.String("error_code", failure.Code))
		return markFailed(ctx, span, j, failure.Code, failure.Message, rawKey)
	}
	if len(receipt.Details) == 0 {
		span.AddFields(logger.String("analysis_status", analysisNoData))
		return markNoData(ctx, j, rawKey)
	}
	span.AddFields(logger.String("analysis_status", analysisSucceeded))
	return register(ctx, span, j, receipt, rawKey)
}

// callOpenAI は OpenAI 呼び出しを "openai_request" span として実行する。所要時間と成否は span_finished に載る。
func callOpenAI(ctx context.Context, client analyze.Client, jpegData []byte) ([]byte, error) {
	ctx, span := logger.StartSpan(ctx, log, "openai_request")
	apiCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	raw, err := client.Analyze(apiCtx, jpegData)
	span.End(err)
	return raw, err
}

func logOpenAIFailure(ctx context.Context, failure *analyze.Failure) {
	// 生レスポンス本文は出さず、OpenAI の error object から調査用フィールドだけを記録する。
	log.Error(ctx, "OpenAI API error", logger.Event("openai_api_error"),
		logger.HTTPStatusCode(failure.HTTPStatus),
		logger.String("provider_type", failure.ProviderType),
		logger.String("provider_code", failure.ProviderCode),
		logger.String("provider_message", truncateMessage(failure.ProviderMessage)),
	)
}

func responseID(raw []byte) string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.ID
}
func safeID(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "response"
	}
	return b.String()
}

func openAISettings(ctx context.Context) (analyze.Client, error) {
	settingsOnce.Do(func() {
		if cfg.OpenAIAPIKeyParameter == "" || cfg.OpenAIModel == "" || cfg.OpenAIReasoningEffort == "" {
			settingsErr = fmt.Errorf("OpenAI configuration is missing")
			return
		}
		out, err := ssmc.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(cfg.OpenAIAPIKeyParameter), WithDecryption: aws.Bool(true)})
		if err != nil {
			settingsErr = err
			return
		}
		settings = analyze.Client{APIKey: aws.ToString(out.Parameter.Value), Model: cfg.OpenAIModel, ReasoningEffort: cfg.OpenAIReasoningEffort}
	})
	return settings, settingsErr
}

func key(j job) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(j.UserID)}, "SK": &ddbtypes.AttributeValueMemberS{Value: app.UploadSK(j.UploadID)}}
}
func terminalValues(j job, status, rawKey string) map[string]ddbtypes.AttributeValue {
	v := map[string]ddbtypes.AttributeValue{":status": &ddbtypes.AttributeValueMemberS{Value: status}, ":analyzing": &ddbtypes.AttributeValueMemberS{Value: "ANALYZING"}, ":attempt": &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(j.Attempt)}, ":now": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)}}
	if rawKey != "" {
		v[":raw_key"] = &ddbtypes.AttributeValueMemberS{Value: rawKey}
	}
	return v
}
func conditional(err error) bool {
	var canceled *ddbtypes.TransactionCanceledException
	if errors.As(err, &canceled) {
		for _, reason := range canceled.CancellationReasons {
			if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
				return true
			}
		}
	}
	var api smithy.APIError
	return errors.As(err, &api) && (api.ErrorCode() == "ConditionalCheckFailedException" || api.ErrorCode() == "TransactionCanceledException" && strings.Contains(api.ErrorMessage(), "ConditionalCheckFailed"))
}

func markAnalyzing(ctx context.Context, j job) (bool, error) {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(cfg.UploadHistoriesTable), Key: key(j), UpdateExpression: aws.String("SET #status=:analyzing, updated_at=:now"), ConditionExpression: aws.String("#status IN (:uploading,:analyzing) AND attempt=:attempt"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":uploading": &ddbtypes.AttributeValueMemberS{Value: "UPLOADING"}, ":analyzing": &ddbtypes.AttributeValueMemberS{Value: "ANALYZING"}, ":attempt": &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(j.Attempt)}, ":now": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)}}})
	if conditional(err) {
		return false, nil
	}
	return err == nil, err
}

func markFailed(ctx context.Context, span *logger.Span, j job, code, msg, rawKey string) error {
	span.AddFields(logger.String("analysis_status", analysisFailed), logger.String("error_code", code))
	v := terminalValues(j, "FAILED", rawKey)
	v[":code"] = &ddbtypes.AttributeValueMemberS{Value: code}
	v[":message"] = &ddbtypes.AttributeValueMemberS{Value: truncateMessage(msg)}
	expr := "SET #status=:status,error_code=:code,error_message=:message,failed_at=:now,updated_at=:now"
	if rawKey != "" {
		expr += ",raw_result_s3_key=:raw_key"
	}
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(cfg.UploadHistoriesTable), Key: key(j), UpdateExpression: aws.String(expr), ConditionExpression: aws.String("#status=:analyzing AND attempt=:attempt"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: v})
	if conditional(err) {
		return nil
	}
	return err
}
func markNoData(ctx context.Context, j job, rawKey string) error {
	v := terminalValues(j, "NO_DATA", rawKey)
	expr := "SET #status=:status,updated_at=:now"
	if rawKey != "" {
		expr += ",raw_result_s3_key=:raw_key"
	}
	expr += " REMOVE error_code,error_message,failed_at"
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(cfg.UploadHistoriesTable), Key: key(j), UpdateExpression: aws.String(expr), ConditionExpression: aws.String("#status=:analyzing AND attempt=:attempt"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: v})
	if conditional(err) {
		return nil
	}
	return err
}
func truncateMessage(s string) string {
	r := []rune(s)
	if len(r) > 500 {
		return string(r[:500])
	}
	return s
}

func register(ctx context.Context, span *logger.Span, j job, r analyze.Receipt, rawKey string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	billingID, err := app.NewID()
	if err != nil {
		return err
	}
	store := ""
	if r.StoreName != nil {
		store = *r.StoreName
	}
	purchased := *r.PurchasedAt
	month := purchased[:7]
	b := billing{PK: app.UserPK(j.UserID), SK: app.BillingSK(billingID), Type: "BILLING", BillingID: billingID, UploadID: j.UploadID, StoreName: store, PurchasedAt: purchased, YearMonth: month, OriginalAmount: *r.TotalAmount, FinalAmount: *r.TotalAmount, Source: "AI", CreatedAt: now, UpdatedAt: now}
	bi, err := attributevalue.MarshalMap(b)
	if err != nil {
		return err
	}
	v := terminalValues(j, "SUCCEEDED", rawKey)
	v[":billing_id"] = &ddbtypes.AttributeValueMemberS{Value: billingID}
	update := "SET #status=:status,billing_id=:billing_id,updated_at=:now"
	if rawKey != "" {
		update += ",raw_result_s3_key=:raw_key"
	}
	items := []ddbtypes.TransactWriteItem{{Update: &ddbtypes.Update{TableName: aws.String(cfg.UploadHistoriesTable), Key: key(j), UpdateExpression: aws.String(update), ConditionExpression: aws.String("#status=:analyzing AND attempt=:attempt"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: v}}, {Put: &ddbtypes.Put{TableName: aws.String(cfg.BillingsTable), Item: bi, ConditionExpression: aws.String("attribute_not_exists(PK)")}}}
	categoryTotals := map[string]int64{}
	for _, x := range r.Details {
		id, e := app.NewID()
		if e != nil {
			return e
		}
		d := detail{PK: app.DetailPK(j.UserID, billingID), SK: app.DetailSK(id), GSI1PK: app.UploadMonthPK(j.UserID, month), GSI1SK: app.DetailMonthSK(x.Amount, purchased, id), Type: "BILLING_DETAIL", DetailID: id, BillingID: billingID, UploadID: j.UploadID, Name: x.Name, Category: x.Category, CategorySource: "AI", Amount: x.Amount, Quantity: x.Quantity, Source: "AI", StoreName: store, PurchasedAt: purchased, YearMonth: month, CreatedAt: now, UpdatedAt: now}
		di, e := attributevalue.MarshalMap(d)
		if e != nil {
			return e
		}
		items = append(items, ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{TableName: aws.String(cfg.BillingDetailsTable), Item: di, ConditionExpression: aws.String("attribute_not_exists(PK)")}})
		categoryTotals[x.Category] += x.Amount
	}
	names := map[string]string{"#type": "type"}
	values := map[string]ddbtypes.AttributeValue{":type": &ddbtypes.AttributeValueMemberS{Value: "MONTHLY_SUMMARY"}, ":user": &ddbtypes.AttributeValueMemberS{Value: j.UserID}, ":month": &ddbtypes.AttributeValueMemberS{Value: month}, ":now": &ddbtypes.AttributeValueMemberS{Value: now}, ":total": &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(*r.TotalAmount, 10)}, ":one": &ddbtypes.AttributeValueMemberN{Value: "1"}, ":count": &ddbtypes.AttributeValueMemberN{Value: strconv.Itoa(len(r.Details))}}
	add := "total_amount :total, billing_count :one, detail_count :count, version :one"
	i := 0
	for c, total := range categoryTotals {
		i++
		n := fmt.Sprintf("#c%d", i)
		val := fmt.Sprintf(":c%d", i)
		names[n] = "category_total_" + c
		values[val] = &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(total, 10)}
		add += ", " + n + " " + val
	}
	expr := "SET #type=if_not_exists(#type,:type),user_id=if_not_exists(user_id,:user),year_month=if_not_exists(year_month,:month),updated_at=:now ADD " + add
	items = append(items, ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{TableName: aws.String(cfg.MonthlySummariesTable), Key: map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(j.UserID)}, "SK": &ddbtypes.AttributeValueMemberS{Value: app.MonthSK(month)}}, UpdateExpression: aws.String(expr), ExpressionAttributeNames: names, ExpressionAttributeValues: values}})
	_, err = ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if conditional(err) {
		return nil
	}
	var canceled *ddbtypes.TransactionCanceledException
	if errors.As(err, &canceled) {
		for _, reason := range canceled.CancellationReasons {
			if aws.ToString(reason.Code) == "ValidationError" {
				return markFailed(ctx, span, j, "INTERNAL", "解析結果を登録できませんでした", rawKey)
			}
		}
	}
	return err
}

func main() {
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		lambda.Start(lambdawrap.HandleEvent(log, handler))
	} else {
		log.Info(context.Background(), "analyze-receipt is a Lambda function")
	}
}
