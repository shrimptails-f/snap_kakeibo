package main

import (
	"context"
	"strconv"
	"time"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libsqs "snap_kakeibo/backend/internal/library/sqs"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	cfg   = app.LoadConfig()
	log   = logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	ddb   *dynamodb.Client
	queue *libsqs.Queue
)

func init() {
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	c, e := awsconfig.Load(context.Background(), oswrapper.New())
	if e != nil {
		panic(e)
	}
	ddb = dynamodb.NewFromConfig(c)
	// 送信時に ctx の trace を traceparent として付けるので、analyze-receipt 側のログが同じ trace_id で繋がる
	queue = libsqs.New(c, log).Queue(cfg.AnalyzeQueueURL)
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	id := req.PathParameters["uploadId"]
	if id == "" {
		return app.Error(400, "uploadId path parameter is required")
	}
	out, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: aws.String(cfg.UploadHistoriesTable), Key: map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(app.FixedUserID)}, "SK": &ddbtypes.AttributeValueMemberS{Value: app.UploadSK(id)}}, UpdateExpression: aws.String("SET #status=:analyzing,attempt=attempt+:one,updated_at=:now REMOVE error_code,error_message,failed_at"), ConditionExpression: aws.String("#status IN (:failed,:no_data,:analyzing)"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":analyzing": &ddbtypes.AttributeValueMemberS{Value: "ANALYZING"}, ":failed": &ddbtypes.AttributeValueMemberS{Value: "FAILED"}, ":no_data": &ddbtypes.AttributeValueMemberS{Value: "NO_DATA"}, ":one": &ddbtypes.AttributeValueMemberN{Value: "1"}, ":now": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)}}, ReturnValues: ddbtypes.ReturnValueUpdatedNew})
	if err != nil {
		return app.Error(409, "upload cannot be retried")
	}
	attempt, _ := strconv.Atoi(out.Attributes["attempt"].(*ddbtypes.AttributeValueMemberN).Value)
	ctx = logger.ContextWith(ctx, logger.UploadID(id), logger.Int("attempt", attempt))
	body := map[string]any{"user_id": app.FixedUserID, "upload_id": id, "attempt": attempt, "trigger": "RETRY"}
	if _, err = queue.SendJSON(ctx, body); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, map[string]any{"upload_id": id, "status": "ANALYZING", "attempt": attempt})
}
func main() { lambda.Start(lambdawrap.Handle(log, handler)) }
