package main

import (
	"context"

	"snap_kakeibo/backend/internal/app"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type billingItem struct {
	BillingID      string `dynamodbav:"billing_id" json:"billing_id"`
	UploadID       string `dynamodbav:"upload_id" json:"upload_id"`
	StoreName      string `dynamodbav:"store_name" json:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at" json:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month" json:"year_month"`
	OriginalAmount int64  `dynamodbav:"original_amount" json:"original_amount"`
	DiscountAmount int64  `dynamodbav:"discount_amount" json:"discount_amount"`
	FinalAmount    int64  `dynamodbav:"final_amount" json:"final_amount"`
}

type detailItem struct {
	DetailID       string `dynamodbav:"detail_id" json:"detail_id"`
	Name           string `dynamodbav:"name" json:"name"`
	Category       string `dynamodbav:"category" json:"category"`
	CategorySource string `dynamodbav:"category_source" json:"category_source"`
	Amount         int64  `dynamodbav:"amount" json:"amount"`
	Quantity       int64  `dynamodbav:"quantity" json:"quantity"`
}

var (
	cfg = app.LoadConfig()
	ddb *dynamodb.Client
)

func init() {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	billingID := req.PathParameters["billingId"]
	if billingID == "" {
		return app.Error(400, "billingId path parameter is required")
	}
	billingOut, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(cfg.BillingsTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(app.FixedUserID)},
			"SK": &ddbtypes.AttributeValueMemberS{Value: app.BillingSK(billingID)},
		},
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	if len(billingOut.Item) == 0 {
		return app.Error(404, "billing not found")
	}
	var billing billingItem
	if err := attributevalue.UnmarshalMap(billingOut.Item, &billing); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}

	detailsOut, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(cfg.BillingDetailsTable),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pk": &ddbtypes.AttributeValueMemberS{Value: app.DetailPK(app.FixedUserID, billingID)},
		},
	})
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	var details []detailItem
	if err := attributevalue.UnmarshalListOfMaps(detailsOut.Items, &details); err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500}, err
	}
	return app.JSON(200, map[string]any{
		"billing": billing,
		"details": details,
	})
}

func main() {
	lambda.Start(handler)
}
