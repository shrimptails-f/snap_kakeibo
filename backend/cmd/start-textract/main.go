package main

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/textract"
	textracttypes "github.com/aws/aws-sdk-go-v2/service/textract/types"
)

var (
	cfg       = app.LoadConfig()
	ddb       *dynamodb.Client
	texClient *textract.Client
)

func init() {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	texClient = textract.NewFromConfig(awsCfg)
}

func handler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		var s3Event events.S3Event
		if err := json.Unmarshal([]byte(record.Body), &s3Event); err != nil {
			return err
		}
		for _, s3Record := range s3Event.Records {
			key, err := url.QueryUnescape(s3Record.S3.Object.Key)
			if err != nil {
				return err
			}
			userID, uploadID, ok := idsFromKey(key)
			if !ok {
				continue
			}
			if err := start(ctx, s3Record.S3.Bucket.Name, key, userID, uploadID); err != nil {
				return err
			}
		}
	}
	return nil
}

func idsFromKey(key string) (string, string, bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != "receipts" || parts[3] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func start(ctx context.Context, bucket, key, userID, uploadID string) error {
	item, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(cfg.UploadHistoriesTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(userID)},
			"SK": &ddbtypes.AttributeValueMemberS{Value: app.UploadSK(uploadID)},
		},
	})
	if err != nil {
		return err
	}
	var history struct {
		Attempt int `dynamodbav:"attempt"`
	}
	if err := attributevalue.UnmarshalMap(item.Item, &history); err != nil {
		return err
	}
	if history.Attempt == 0 {
		history.Attempt = 1
	}

	attempt := strconv.Itoa(history.Attempt)
	input := &textract.StartExpenseAnalysisInput{
		ClientRequestToken: aws.String(uploadID + "_" + attempt),
		DocumentLocation: &textracttypes.DocumentLocation{
			S3Object: &textracttypes.S3Object{
				Bucket: aws.String(bucket),
				Name:   aws.String(key),
			},
		},
		JobTag: aws.String(app.JobTag(userID, uploadID, attempt)),
		NotificationChannel: &textracttypes.NotificationChannel{
			RoleArn:     aws.String(cfg.TextractRoleARN),
			SNSTopicArn: aws.String(cfg.TextractTopicARN),
		},
	}
	out, err := texClient.StartExpenseAnalysis(ctx, input)
	if err != nil {
		// Textract のエラーは本文が空のことがあるので、何を投げたかを一緒に残す
		log.Printf("StartExpenseAnalysis failed: %v request=%s", err, describeStartInput(input))
		return err
	}
	log.Printf("StartExpenseAnalysis started: job_id=%s upload_id=%s attempt=%s", aws.ToString(out.JobId), uploadID, attempt)

	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(cfg.UploadHistoriesTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(userID)},
			"SK": &ddbtypes.AttributeValueMemberS{Value: app.UploadSK(uploadID)},
		},
		UpdateExpression: aws.String("SET #status = :status, textract_job_id = :job_id, updated_at = :updated_at"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":status":     &ddbtypes.AttributeValueMemberS{Value: "ANALYZING"},
			":job_id":     &ddbtypes.AttributeValueMemberS{Value: aws.ToString(out.JobId)},
			":updated_at": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	return err
}

// describeStartInput はログ用に StartExpenseAnalysisInput を1行の JSON にする。
func describeStartInput(in *textract.StartExpenseAnalysisInput) string {
	b, err := json.Marshal(map[string]any{
		"client_request_token": aws.ToString(in.ClientRequestToken),
		"bucket":               aws.ToString(in.DocumentLocation.S3Object.Bucket),
		"key":                  aws.ToString(in.DocumentLocation.S3Object.Name),
		"job_tag":              aws.ToString(in.JobTag),
		"role_arn":             aws.ToString(in.NotificationChannel.RoleArn),
		"sns_topic_arn":        aws.ToString(in.NotificationChannel.SNSTopicArn),
		"region":               texClient.Options().Region,
	})
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func main() {
	lambda.Start(handler)
}
