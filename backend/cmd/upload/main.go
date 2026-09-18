package main

import (
	"context"
	"encoding/json"
	"time"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	cfg       = app.LoadConfig()
	ddb       *dynamodb.Client
	presigner *s3.PresignClient
)

func init() {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	presigner = s3.NewPresignClient(s3.NewFromConfig(awsCfg))
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

	put, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(cfg.ReceiptBucket),
		Key:         aws.String(s3Key),
		ContentType: aws.String(in.ContentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}

	return app.JSON(200, response{
		UploadID:  uploadID,
		S3Key:     s3Key,
		PutURL:    put.URL,
		ExpiresAt: expiresAt,
	})
}

func main() {
	lambda.Start(handler)
}
