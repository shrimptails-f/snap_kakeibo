package main

import (
	"context"
	"encoding/json"
	"time"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/lambdawrap"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	libs3 "snap_kakeibo/backend/internal/library/s3"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type request struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
}

type response struct {
	UploadID  string `json:"upload_id"`
	S3Key     string `json:"s3_key"`
	PutURL    string `json:"put_url"`
	ExpiresAt string `json:"expires_at"`
}

type uploadHistory struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	GSI1PK      string `dynamodbav:"GSI1PK"`
	GSI1SK      string `dynamodbav:"GSI1SK"`
	Type        string `dynamodbav:"type"`
	UploadID    string `dynamodbav:"upload_id"`
	Status      string `dynamodbav:"status"`
	Attempt     int    `dynamodbav:"attempt"`
	S3Key       string `dynamodbav:"s3_key"`
	FileName    string `dynamodbav:"file_name"`
	ContentType string `dynamodbav:"content_type"`
	YearMonth   string `dynamodbav:"year_month"`
	ExpiresAt   string `dynamodbav:"expires_at"`
	CreatedAt   string `dynamodbav:"created_at"`
	UpdatedAt   string `dynamodbav:"updated_at"`
}

var (
	cfg      = app.LoadConfig()
	log      = logger.New(logger.Options{Level: cfg.LogLevel, Service: lambdacontext.FunctionName, Environment: cfg.Stage})
	ddb      *dynamodb.Client
	receipts *libs3.Bucket
)

func init() {
	// STAGE=local / ci なら Floci、それ以外は AWS を向く
	awsCfg, err := awsconfig.Load(context.Background(), oswrapper.New())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	receipts = libs3.New(awsCfg, log).Bucket(cfg.ReceiptBucket)
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	if err := app.Required(cfg.UploadHistoriesTable, "UPLOAD_HISTORIES_TABLE"); err != nil {
		return app.Error(500, err.Error())
	}
	if err := app.Required(cfg.ReceiptBucket, "RECEIPT_BUCKET"); err != nil {
		return app.Error(500, err.Error())
	}

	var in request
	if req.Body != "" {
		if err := json.Unmarshal([]byte(req.Body), &in); err != nil {
			return app.Error(400, "invalid JSON body")
		}
	}
	if in.ContentType == "" {
		in.ContentType = "image/jpeg"
	}
	if in.FileName == "" {
		in.FileName = "receipt.jpg"
	}

	uploadID, err := app.NewID()
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	now := time.Now().UTC()
	createdAt := now.Format(time.RFC3339)
	expiresAt := now.Add(15 * time.Minute).Format(time.RFC3339)
	month := app.YearMonth(now)
	s3Key := "receipts/" + app.FixedUserID + "/" + uploadID + "/original.jpg"

	history := uploadHistory{
		PK:          app.UserPK(app.FixedUserID),
		SK:          app.UploadSK(uploadID),
		GSI1PK:      app.UploadMonthPK(app.FixedUserID, month),
		GSI1SK:      app.UploadMonthSK(createdAt, uploadID),
		Type:        "UPLOAD_HISTORY",
		UploadID:    uploadID,
		Status:      "UPLOADING",
		Attempt:     1,
		S3Key:       s3Key,
		FileName:    in.FileName,
		ContentType: in.ContentType,
		YearMonth:   month,
		ExpiresAt:   expiresAt,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	item, err := attributevalue.MarshalMap(history)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(cfg.UploadHistoriesTable),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK) AND attribute_not_exists(SK)"),
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}

	putURL, err := receipts.PresignPutObject(ctx, s3Key, in.ContentType, 15*time.Minute)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}

	return app.JSON(200, response{
		UploadID:  uploadID,
		S3Key:     s3Key,
		PutURL:    putURL,
		ExpiresAt: expiresAt,
	})
}

func main() {
	lambda.Start(lambdawrap.Handle(log, handler))
}
