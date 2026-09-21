package main

import (
	"context"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/library/queue"
	"snap_kakeibo/backend/internal/analysis/library/settings"
	"snap_kakeibo/backend/internal/di"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libsqs "snap_kakeibo/backend/internal/library/sqs"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
)

// eventQueueMessageInvalid はキューのメッセージをジョブに変換できなかったときの event 値。
const eventQueueMessageInvalid = "queue_message_invalid"

var (
	analyze application.AnalyzeReceiptUsecaseInterface
	// receiptBucket は RETRY 形式のジョブの画像バケット。S3 通知は通知自身がバケットを運ぶ
	receiptBucket string
	log           logger.Interface
)

func init() {
	osw := oswrapper.New()
	cfg, err := settings.Load(osw)
	if err != nil {
		panic(err)
	}
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		panic(err)
	}
	configuredLogger := logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	container, err := di.NewAnalyzeReceiptContainer(cfg, awsCfg, osw, configuredLogger)
	if err != nil {
		panic(err)
	}
	log, err = di.ResolveLogger(container)
	if err != nil {
		panic(err)
	}
	analyze, err = di.ResolveAnalyzeReceiptUsecase(container)
	if err != nil {
		panic(err)
	}
	receiptBucket = cfg.ReceiptBucket
}

// handler は SQS レコードごとにジョブを取り出し、順に解析する。
// ジョブが error を返したらそこで止めて Lambda を失敗させ、レコード全体の再配信に任せる
// (処理済みのジョブは analysis_requests の状態で弾かれる)。
func handler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		// レコードが運んできた trace を引き継ぎ、message_id を積む
		rctx := libsqs.RecordContext(ctx, record)
		jobs, err := queue.DecodeJobs(record.Body, receiptBucket)
		if err != nil {
			log.Error(rctx, "failed to decode queue message", logger.Event(eventQueueMessageInvalid), logger.Err(err))
			return err
		}
		for _, job := range jobs {
			if _, err := analyze.Analyze(rctx, application.AnalyzeReceiptInput{Job: job}); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() { lambda.Start(lambdawrap.HandleEvent(log, handler)) }
