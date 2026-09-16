package main

import (
	"context"
	"encoding/json"
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
	out, err := texClient.StartExpenseAnalysis(ctx, &textract.StartExpenseAnalysisInput{
		ClientRequestToken: aws.String(uploadID + "#" + attempt),
		DocumentLocation: &textracttypes.DocumentLocation{
			S3Object: &textracttypes.S3Object{
				Bucket: aws.String(bucket),
				Name:   aws.String(key),
			},
		},
		JobTag: aws.String(userID + "#" + uploadID + "#" + attempt),
		NotificationChannel: &textracttypes.NotificationChannel{
			RoleArn:     aws.String(cfg.TextractRoleARN),
			SNSTopicArn: aws.String(cfg.TextractTopicARN),
		},
	})
	if err != nil {
		return err
	}

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

func main() {
	lambda.Start(handler)
}
