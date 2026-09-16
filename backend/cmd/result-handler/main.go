package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"regexp"
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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/textract"
	textracttypes "github.com/aws/aws-sdk-go-v2/service/textract/types"
)

type notification struct {
	JobID  string `json:"JobId"`
	Status string `json:"Status"`
	JobTag string `json:"JobTag"`
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
	OriginalAmount int64  `dynamodbav:"original_amount"`
	DiscountAmount int64  `dynamodbav:"discount_amount"`
	FinalAmount    int64  `dynamodbav:"final_amount"`
	Source         string `dynamodbav:"source"`
	IsEdited       bool   `dynamodbav:"is_edited"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
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
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
	Source         string `dynamodbav:"source"`
	IsEdited       bool   `dynamodbav:"is_edited"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
}

var (
	cfg       = app.LoadConfig()
	ddb       *dynamodb.Client
	s3Client  *s3.Client
	texClient *textract.Client
)

func init() {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	ddb = dynamodb.NewFromConfig(awsCfg)
	s3Client = s3.NewFromConfig(awsCfg)
	texClient = textract.NewFromConfig(awsCfg)
}

func handler(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		var n notification
		if err := json.Unmarshal([]byte(record.Body), &n); err != nil {
			return err
		}
		userID, uploadID, attempt, ok := app.SplitJobTag(n.JobTag)
		if !ok {
			continue
		}
		if n.Status != string(textracttypes.JobStatusSucceeded) {
			if err := markFailed(ctx, userID, uploadID, "TEXTRACT_"+n.Status, "Textract job did not succeed"); err != nil {
				return err
			}
			continue
		}
		if err := handleSuccess(ctx, n.JobID, userID, uploadID, attempt); err != nil {
			return err
		}
	}
	return nil
}

func handleSuccess(ctx context.Context, jobID, userID, uploadID, _ string) error {
	docs, raw, err := getAllExpenseDocuments(ctx, jobID)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	rawKey := "textract-results/" + userID + "/" + uploadID + "/result.json"
	if _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(cfg.ReceiptBucket),
		Key:         aws.String(rawKey),
		Body:        bytes.NewReader(raw),
		ContentType: aws.String("application/json"),
	}); err != nil {
		return err
	}

	extracted := extract(docs)
	if len(extracted.lines) == 0 {
		return updateUpload(ctx, userID, uploadID, map[string]ddbtypes.AttributeValue{
			":status":  &ddbtypes.AttributeValueMemberS{Value: "NO_DATA"},
			":raw_key": &ddbtypes.AttributeValueMemberS{Value: rawKey},
			":now":     &ddbtypes.AttributeValueMemberS{Value: now},
		}, "SET #status = :status, raw_textract_s3_key = :raw_key, updated_at = :now")
	}

	if extracted.total <= 0 {
		var lineTotal int64
		for _, line := range extracted.lines {
			lineTotal += line.amount
		}
		extracted.total = lineTotal
	}
	if extracted.date == "" {
		extracted.date = time.Now().UTC().Format("2006-01-02")
	}
	month := app.YearMonthFromDate(extracted.date)
	billingID, err := app.NewID()
	if err != nil {
		return err
	}

	b := billing{
		PK:             app.UserPK(userID),
		SK:             app.BillingSK(billingID),
		Type:           "BILLING",
		BillingID:      billingID,
		UploadID:       uploadID,
		StoreName:      extracted.store,
		PurchasedAt:    extracted.date,
		YearMonth:      month,
		OriginalAmount: extracted.total,
		DiscountAmount: 0,
		FinalAmount:    extracted.total,
		Source:         "TEXTRACT",
		IsEdited:       false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if b.StoreName == "" {
		b.StoreName = "unknown"
	}
	item, err := attributevalue.MarshalMap(b)
	if err != nil {
		return err
	}
	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(cfg.BillingsTable), Item: item}); err != nil {
		return err
	}

	var detailTotal int64
	for _, line := range extracted.lines {
		detailID, err := app.NewID()
		if err != nil {
			return err
		}
		d := detail{
			PK:             app.DetailPK(userID, billingID),
			SK:             app.DetailSK(detailID),
			GSI1PK:         app.UploadMonthPK(userID, month),
			GSI1SK:         app.DetailMonthSK(line.amount, extracted.date, detailID),
			Type:           "BILLING_DETAIL",
			DetailID:       detailID,
			BillingID:      billingID,
			UploadID:       uploadID,
			Name:           line.name,
			Category:       "unknown",
			CategorySource: "UNKNOWN",
			Amount:         line.amount,
			Quantity:       1,
			Source:         "TEXTRACT",
			IsEdited:       false,
			StoreName:      b.StoreName,
			PurchasedAt:    extracted.date,
			YearMonth:      month,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if d.Name == "" {
			d.Name = "明細"
		}
		item, err := attributevalue.MarshalMap(d)
		if err != nil {
			return err
		}
		if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(cfg.BillingDetailsTable), Item: item}); err != nil {
			return err
		}
		detailTotal += line.amount
	}

	if err := updateMonthlySummary(ctx, userID, month, b.FinalAmount, int64(len(extracted.lines)), detailTotal, now); err != nil {
		return err
	}
	return updateUpload(ctx, userID, uploadID, map[string]ddbtypes.AttributeValue{
		":status":     &ddbtypes.AttributeValueMemberS{Value: "SUCCEEDED"},
		":billing_id": &ddbtypes.AttributeValueMemberS{Value: billingID},
		":raw_key":    &ddbtypes.AttributeValueMemberS{Value: rawKey},
		":now":        &ddbtypes.AttributeValueMemberS{Value: now},
	}, "SET #status = :status, billing_id = :billing_id, raw_textract_s3_key = :raw_key, updated_at = :now")
}

func getAllExpenseDocuments(ctx context.Context, jobID string) ([]textracttypes.ExpenseDocument, []byte, error) {
	var docs []textracttypes.ExpenseDocument
	var pages []textract.GetExpenseAnalysisOutput
	var next *string
	for {
		out, err := texClient.GetExpenseAnalysis(ctx, &textract.GetExpenseAnalysisInput{
			JobId:     aws.String(jobID),
			NextToken: next,
		})
		if err != nil {
			return nil, nil, err
		}
		docs = append(docs, out.ExpenseDocuments...)
		pages = append(pages, *out)
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	raw, err := json.MarshalIndent(pages, "", "  ")
	return docs, raw, err
}

type line struct {
	name   string
	amount int64
}

type extractedReceipt struct {
	store string
	date  string
	total int64
	lines []line
}

func extract(docs []textracttypes.ExpenseDocument) extractedReceipt {
	var out extractedReceipt
	for _, doc := range docs {
		for _, field := range doc.SummaryFields {
			typ := fieldType(field)
			value := fieldValue(field)
			switch typ {
			case "TOTAL", "AMOUNT_DUE":
				if amount := parseAmount(value); amount > out.total {
					out.total = amount
				}
			case "INVOICE_RECEIPT_DATE":
				if out.date == "" {
					out.date = normalizeDate(value)
				}
			case "VENDOR_NAME", "RECEIVER_NAME":
				if out.store == "" {
					out.store = value
				}
			}
		}
		for _, group := range doc.LineItemGroups {
			for _, li := range group.LineItems {
				var name string
				var amount int64
				for _, field := range li.LineItemExpenseFields {
					switch fieldType(field) {
					case "ITEM":
						name = fieldValue(field)
					case "PRICE", "EXPENSE_ROW", "TOTAL_PRICE":
						if parsed := parseAmount(fieldValue(field)); parsed > 0 {
							amount = parsed
						}
					}
				}
				if amount > 0 {
					out.lines = append(out.lines, line{name: name, amount: amount})
				}
			}
		}
	}
	return out
}

func fieldType(field textracttypes.ExpenseField) string {
	if field.Type == nil || field.Type.Text == nil {
		return ""
	}
	return strings.ToUpper(aws.ToString(field.Type.Text))
}

func fieldValue(field textracttypes.ExpenseField) string {
	if field.ValueDetection == nil || field.ValueDetection.Text == nil {
		return ""
	}
	return strings.TrimSpace(aws.ToString(field.ValueDetection.Text))
}

var amountRE = regexp.MustCompile(`[-+]?[0-9][0-9,]*(\.[0-9]+)?`)

func parseAmount(s string) int64 {
	match := amountRE.FindString(strings.ReplaceAll(s, " ", ""))
	if match == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(match, ",", ""), 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f))
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	layouts := []string{"2006-01-02", "2006/01/02", "2006.01.02", "01/02/2006", "1/2/2006", "2006年1月2日"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func updateMonthlySummary(ctx context.Context, userID, month string, total, detailCount, detailTotal int64, now string) error {
	key := map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(userID)},
		"SK": &ddbtypes.AttributeValueMemberS{Value: app.MonthSK(month)},
	}
	current, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(cfg.MonthlySummariesTable),
		Key:       key,
	})
	if err != nil {
		return err
	}
	var summary struct {
		CategoryTotals map[string]int64 `dynamodbav:"category_totals"`
	}
	if len(current.Item) > 0 {
		if err := attributevalue.UnmarshalMap(current.Item, &summary); err != nil {
			return err
		}
	}
	if summary.CategoryTotals == nil {
		summary.CategoryTotals = map[string]int64{}
	}
	summary.CategoryTotals["unknown"] += detailTotal
	categoryTotals, err := attributevalue.MarshalMap(summary.CategoryTotals)
	if err != nil {
		return err
	}

	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(cfg.MonthlySummariesTable),
		Key:       key,
		UpdateExpression: aws.String("SET #type = if_not_exists(#type, :type), user_id = :user_id, year_month = :month, updated_at = :now, category_totals = :category_totals ADD total_amount :total, billing_count :one, detail_count :detail_count, version :one"),
		ExpressionAttributeNames: map[string]string{
			"#type": "type",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":type":   &ddbtypes.AttributeValueMemberS{Value: "MONTHLY_SUMMARY"},
			":user_id": &ddbtypes.AttributeValueMemberS{Value: userID},
			":month":  &ddbtypes.AttributeValueMemberS{Value: month},
			":now":    &ddbtypes.AttributeValueMemberS{Value: now},
			":category_totals": &ddbtypes.AttributeValueMemberM{Value: categoryTotals},
			":total":        &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(total, 10)},
			":one":          &ddbtypes.AttributeValueMemberN{Value: "1"},
			":detail_count": &ddbtypes.AttributeValueMemberN{Value: strconv.FormatInt(detailCount, 10)},
		},
	})
	return err
}

func updateUpload(ctx context.Context, userID, uploadID string, values map[string]ddbtypes.AttributeValue, expression string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(cfg.UploadHistoriesTable),
		Key: map[string]ddbtypes.AttributeValue{
			"PK": &ddbtypes.AttributeValueMemberS{Value: app.UserPK(userID)},
			"SK": &ddbtypes.AttributeValueMemberS{Value: app.UploadSK(uploadID)},
		},
		UpdateExpression: aws.String(expression),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: values,
	})
	return err
}

func markFailed(ctx context.Context, userID, uploadID, code, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return updateUpload(ctx, userID, uploadID, map[string]ddbtypes.AttributeValue{
		":status":  &ddbtypes.AttributeValueMemberS{Value: "FAILED"},
		":code":    &ddbtypes.AttributeValueMemberS{Value: code},
		":message": &ddbtypes.AttributeValueMemberS{Value: message},
		":now":     &ddbtypes.AttributeValueMemberS{Value: now},
	}, "SET #status = :status, error_code = :code, error_message = :message, failed_at = :now, updated_at = :now")
}

func main() {
	lambda.Start(handler)
}
