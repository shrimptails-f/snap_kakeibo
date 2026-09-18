package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

var cfg = app.LoadConfig()
var ddb *dynamodb.Client
var sqsc *sqs.Client

func init() {
	c, e := awsconfig.LoadDefaultConfig(context.Background())
	if e != nil {
		panic(e)
	}
	ddb = dynamodb.NewFromConfig(c)
	sqsc = sqs.NewFromConfig(c)
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
	body, _ := json.Marshal(map[string]any{"user_id": app.FixedUserID, "upload_id": id, "attempt": attempt, "trigger": "RETRY"})
	if _, err = sqsc.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(cfg.AnalyzeQueueURL), MessageBody: aws.String(string(body))}); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, map[string]any{"upload_id": id, "status": "ANALYZING", "attempt": attempt})
}
func main() { lambda.Start(handler) }
