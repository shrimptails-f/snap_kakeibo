package main

import (
	"context"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/auth"
	libawsconfig "snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/oswrapper"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type uploadItem struct {
	UploadID    string `dynamodbav:"upload_id" json:"upload_id"`
	BillingID   string `dynamodbav:"billing_id" json:"billing_id,omitempty"`
	Status      string `dynamodbav:"status" json:"status"`
	FileName    string `dynamodbav:"file_name" json:"file_name"`
	ContentType string `dynamodbav:"content_type" json:"content_type"`
	YearMonth   string `dynamodbav:"year_month" json:"year_month"`
	ErrorCode   string `dynamodbav:"error_code" json:"error_code,omitempty"`
	ErrorMsg    string `dynamodbav:"error_message" json:"error_message,omitempty"`
	CreatedAt   string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt   string `dynamodbav:"updated_at" json:"updated_at"`
}

var (
	cfg     = app.LoadConfig()
	ddb     *dynamodb.Client
	authSvc *auth.Service
)

func init() {
	awsCfg, err := libawsconfig.Load(context.Background(), oswrapper.New())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	authSvc = &auth.Service{DDB: ddb, SSM: ssm.NewFromConfig(awsCfg), Cfg: cfg}
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	month := req.PathParameters["month"]
	if month == "" {
		return app.Error(400, "month path parameter is required")
	}
	claims, err := authSvc.VerifyRequest(ctx, req)
	if err != nil {
		return app.Error(401, "unauthorized")
	}
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(cfg.UploadHistoriesTable),
		IndexName:              aws.String("upload_month_index"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pk": &ddbtypes.AttributeValueMemberS{Value: app.UploadMonthPK(claims.UserID, month)},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	var items []uploadItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, map[string]any{"items": items})
}

func main() {
	lambda.Start(handler)
}
