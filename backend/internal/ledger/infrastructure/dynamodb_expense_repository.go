// Package infrastructure は家計簿コンテキストの application が定義した interface の DynamoDB 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// expenseItem は expenses の項目のうち支出集約の復元に使う属性。analyze-receipt が書く属性名と揃える。
// recorded_amount は保存されているが、集約は read_amount + adjustment_amount から再導出するので読まない。
type expenseItem struct {
	ExpenseID         string `dynamodbav:"expense_id"`
	AnalysisRequestID string `dynamodbav:"analysis_request_id"`
	StoreName         string `dynamodbav:"store_name"`
	PurchaseDate      string `dynamodbav:"purchase_date"`
	YearMonth         string `dynamodbav:"year_month"`
	ReadAmount        int64  `dynamodbav:"read_amount"`
	AnalysisEvidence  string `dynamodbav:"analysis_evidence"`
	AdjustmentAmount  int64  `dynamodbav:"adjustment_amount"`
	IsEdited          bool   `dynamodbav:"is_edited"`
	Source            string `dynamodbav:"source"`
	UpdatedAt         string `dynamodbav:"updated_at"`
}

// expenseDetailItem は expense_details の項目のうち支出明細の復元に使う属性。
type expenseDetailItem struct {
	ExpenseID         string `dynamodbav:"expense_id"`
	DetailID          string `dynamodbav:"detail_id"`
	Name              string `dynamodbav:"name"`
	Category          string `dynamodbav:"category"`
	CategorySource    string `dynamodbav:"category_source"`
	Amount            int64  `dynamodbav:"amount"`
	TaxIncludedAmount *int64 `dynamodbav:"tax_included_amount"`
	TaxRate           *int64 `dynamodbav:"tax_rate"`
	TaxMode           string `dynamodbav:"tax_mode"`
	TaxAllocation     string `dynamodbav:"tax_allocation"`
	TaxStatus         string `dynamodbav:"tax_status,omitempty"`
	TaxReason         string `dynamodbav:"tax_reason,omitempty"`
	SuggestedTaxRate  *int64 `dynamodbav:"suggested_tax_rate,omitempty"`
	Quantity          int64  `dynamodbav:"quantity"`
	Source            string `dynamodbav:"source"`
	IsEdited          bool   `dynamodbav:"is_edited"`
	StoreName         string `dynamodbav:"store_name"`
	PurchaseDate      string `dynamodbav:"purchase_date"`
}

// DynamoDBExpenseRepository は expenses と expense_details から支出集約を復元する。
type DynamoDBExpenseRepository struct {
	Client         *libdynamodb.Client
	Expenses       *libdynamodb.Table
	ExpenseDetails *libdynamodb.Table
}

var _ application.ExpenseFinder = DynamoDBExpenseRepository{}
var _ application.ExpenseRepository = DynamoDBExpenseRepository{}
var _ application.MonthlyExpenseLister = DynamoDBExpenseRepository{}

// ListByMonth は月別 GSI を内部で最後まで読み、GSI1SK の昇順(金額降順)を保って返す。
func (r DynamoDBExpenseRepository) ListByMonth(ctx context.Context, userID common.UserID, month domain.YearMonth) ([]application.MonthlyExpenseItem, error) {
	items := make([]application.MonthlyExpenseItem, 0)
	var cursor map[string]ddbtypes.AttributeValue
	for {
		out, err := r.ExpenseDetails.Query(ctx, &awssdk.QueryInput{
			IndexName: aws.String("detail_month_amount_index"), KeyConditionExpression: aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UserMonthPK(userID.String(), month.String()))},
			ExclusiveStartKey:         cursor,
		})
		if err != nil {
			return nil, err
		}
		var records []expenseDetailItem
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &records); err != nil {
			return nil, fmt.Errorf("unmarshal monthly expense details: %w", err)
		}
		for _, record := range records {
			detailID, ok := domain.NewExpenseDetailID(record.DetailID)
			if !ok {
				return nil, fmt.Errorf("restore monthly expense detail: invalid detail ID")
			}
			expenseID, ok := common.NewExpenseID(record.ExpenseID)
			if !ok {
				return nil, fmt.Errorf("restore monthly expense detail: invalid expense ID")
			}
			category, err := common.NewCategory(record.Category)
			if err != nil {
				return nil, fmt.Errorf("restore monthly expense detail category: %w", err)
			}
			source, err := domain.NewRecordSource(record.Source)
			if err != nil {
				return nil, fmt.Errorf("restore monthly expense detail source: %w", err)
			}
			if _, err := common.NewPurchaseDate(record.PurchaseDate); err != nil {
				return nil, fmt.Errorf("restore monthly expense purchase date: %w", err)
			}
			items = append(items, application.MonthlyExpenseItem{
				DetailID: detailID, ExpenseID: expenseID, Name: record.Name, Category: category,
				Amount: record.Amount, TaxIncludedAmount: record.TaxIncludedAmount, TaxRate: record.TaxRate, TaxMode: record.TaxMode, Quantity: record.Quantity, Source: source, IsEdited: record.IsEdited,
				StoreName: record.StoreName, PurchaseDate: record.PurchaseDate,
			})
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		cursor = out.LastEvaluatedKey
	}
	return items, nil
}

// FindByID は利用者と expense_id のキーで支出を GetItem し、支出のパーティション(USER#{user_id}#EXPENSE#{expense_id})を
// Query して支出明細を detail_id の昇順(ULID の採番順)で集約へ復元する。
// 項目が無ければ ErrExpenseNotFound。他人の expense_id は利用者のパーティションに無いので同じく ErrExpenseNotFound になる。
// 1 支出の明細は最大 50 件で 1 回の Query に収まるのでページネーションはしない。
func (r DynamoDBExpenseRepository) FindByID(ctx context.Context, userID common.UserID, expenseID domain.ExpenseID) (domain.Expense, error) {
	out, err := r.Expenses.GetItem(ctx, &awssdk.GetItemInput{
		Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID.String())), "SK": stringValue(ExpenseSK(expenseID.String()))},
	})
	if err != nil {
		return domain.Expense{}, err
	}
	if len(out.Item) == 0 {
		return domain.Expense{}, application.ErrExpenseNotFound
	}
	var item expenseItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Expense{}, fmt.Errorf("unmarshal expense: %w", err)
	}
	details, err := r.listDetails(ctx, userID, expenseID)
	if err != nil {
		return domain.Expense{}, err
	}
	return restoreExpense(userID, expenseID, item, details)
}

func (r DynamoDBExpenseRepository) listDetails(ctx context.Context, userID common.UserID, expenseID domain.ExpenseID) ([]domain.ExpenseDetail, error) {
	out, err := r.ExpenseDetails.Query(ctx, &awssdk.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(DetailPK(userID.String(), expenseID.String()))},
	})
	if err != nil {
		return nil, err
	}
	var items []expenseDetailItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("unmarshal expense details: %w", err)
	}
	details := make([]domain.ExpenseDetail, 0, len(items))
	for _, item := range items {
		detail, err := restoreDetail(item)
		if err != nil {
			return nil, fmt.Errorf("restore expense detail %s: %w", item.DetailID, err)
		}
		details = append(details, detail)
	}
	return details, nil
}

// FindByMonth は支出テーブルを利用者単位で、明細テーブルを月別 GSI で読み、対象月の集約を復元する。
func (r DynamoDBExpenseRepository) FindByMonth(ctx context.Context, userID common.UserID, month domain.YearMonth) ([]domain.Expense, error) {
	expenseOut, err := r.Expenses.Query(ctx, &awssdk.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UserPK(userID.String())), ":sk": stringValue("EXPENSE#")},
	})
	if err != nil {
		return nil, err
	}
	var expenseItems []expenseItem
	if err := attributevalue.UnmarshalListOfMaps(expenseOut.Items, &expenseItems); err != nil {
		return nil, fmt.Errorf("unmarshal expenses: %w", err)
	}
	detailOut, err := r.ExpenseDetails.Query(ctx, &awssdk.QueryInput{
		IndexName: aws.String("detail_month_amount_index"), KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UserMonthPK(userID.String(), month.String()))},
	})
	if err != nil {
		return nil, err
	}
	var detailItems []expenseDetailItem
	if err := attributevalue.UnmarshalListOfMaps(detailOut.Items, &detailItems); err != nil {
		return nil, fmt.Errorf("unmarshal monthly expense details: %w", err)
	}
	grouped := make(map[string][]domain.ExpenseDetail)
	for _, item := range detailItems {
		detail, err := restoreDetail(item)
		if err != nil {
			return nil, fmt.Errorf("restore expense detail %s: %w", item.DetailID, err)
		}
		grouped[item.ExpenseID] = append(grouped[item.ExpenseID], detail)
	}
	expenses := make([]domain.Expense, 0, len(expenseItems))
	for _, item := range expenseItems {
		if item.YearMonth != month.String() {
			continue
		}
		expenseID, ok := common.NewExpenseID(item.ExpenseID)
		if !ok {
			return nil, fmt.Errorf("restore expense: invalid expense ID")
		}
		expense, err := restoreExpense(userID, expenseID, item, grouped[item.ExpenseID])
		if err != nil {
			return nil, err
		}
		expenses = append(expenses, expense)
	}
	return expenses, nil
}

// Save は支出と全明細を一つの DynamoDB transaction で更新する。
func (r DynamoDBExpenseRepository) Save(ctx context.Context, expense domain.Expense, previousDetails []domain.ExpenseDetail, updatedAt time.Time) error {
	if r.Client == nil {
		return fmt.Errorf("save expense: dynamodb client is nil")
	}
	timestamp := updatedAt.UTC().Format(time.RFC3339)
	userID, expenseID := expense.UserID().String(), expense.ID().String()
	values := map[string]ddbtypes.AttributeValue{
		":store": stringValue(expense.StoreName()), ":date": stringValue(expense.PurchaseDate().String()),
		":month": stringValue(expense.PurchaseDate().YearMonth().String()), ":adjustment": numberValue(expense.AdjustmentAmount().Yen()),
		":recorded": numberValue(expense.RecordedAmount().Yen()), ":source": stringValue(expense.Source().String()),
		":edited": boolValue(expense.Edited()), ":updated": stringValue(timestamp),
	}
	items := []ddbtypes.TransactWriteItem{{Update: &ddbtypes.Update{
		TableName: aws.String(r.Expenses.Name()), Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID)), "SK": stringValue(ExpenseSK(expenseID))},
		UpdateExpression:    aws.String("SET store_name=:store,purchase_date=:date,year_month=:month,adjustment_amount=:adjustment,recorded_amount=:recorded,#source=:source,is_edited=:edited,updated_at=:updated"),
		ConditionExpression: aws.String("attribute_exists(PK)"), ExpressionAttributeNames: map[string]string{"#source": "source"}, ExpressionAttributeValues: values,
	}}}
	previous := make(map[domain.ExpenseDetailID]struct{}, len(previousDetails))
	current := make(map[domain.ExpenseDetailID]struct{}, len(expense.Details()))
	for _, detail := range previousDetails {
		previous[detail.ID()] = struct{}{}
	}
	for _, detail := range expense.Details() {
		current[detail.ID()] = struct{}{}
		values := map[string]ddbtypes.AttributeValue{
			":gpk": stringValue(UserMonthPK(userID, expense.PurchaseDate().YearMonth().String())), ":gsk": stringValue(DetailMonthSK(detail.ReportingAmount(), expense.PurchaseDate().String(), detail.ID().String())),
			":name": stringValue(detail.Name()), ":category": stringValue(detail.Category().String()), ":categorySource": stringValue(detail.CategorySource().String()),
			":amount": numberValue(detail.Amount().Yen()), ":quantity": numberValue(detail.Quantity().Int64()), ":source": stringValue(detail.Source().String()),
			":taxStatus": stringValue(detail.TaxStatus()), ":taxReason": stringValue(detail.TaxReason()),
			":taxMode": stringValue(detail.TaxMode()), ":taxAllocation": stringValue(detail.TaxAllocation()),
			":store": stringValue(expense.StoreName()), ":date": stringValue(expense.PurchaseDate().String()), ":month": stringValue(expense.PurchaseDate().YearMonth().String()),
			":edited": boolValue(detail.Edited()), ":updated": stringValue(timestamp),
		}
		if _, exists := previous[detail.ID()]; !exists {
			item, err := attributevalue.MarshalMap(map[string]any{
				"PK": DetailPK(userID, expenseID), "SK": DetailSK(detail.ID().String()),
				"GSI1PK": UserMonthPK(userID, expense.PurchaseDate().YearMonth().String()),
				"GSI1SK": DetailMonthSK(detail.ReportingAmount(), expense.PurchaseDate().String(), detail.ID().String()),
				"type":   "EXPENSE_DETAIL", "detail_id": detail.ID().String(), "expense_id": expenseID,
				"analysis_request_id": expense.SourceRequestID().String(), "name": detail.Name(),
				"category": detail.Category().String(), "category_source": detail.CategorySource().String(),
				"amount": detail.Amount().Yen(), "quantity": detail.Quantity().Int64(),
				"tax_included_amount": detail.TaxIncludedAmount(), "tax_rate": detail.TaxRate(), "tax_mode": detail.TaxMode(), "tax_allocation": detail.TaxAllocation(), "tax_status": detail.TaxStatus(), "tax_reason": detail.TaxReason(), "suggested_tax_rate": detail.SuggestedTaxRate(),
				"source": detail.Source().String(), "store_name": expense.StoreName(),
				"purchase_date": expense.PurchaseDate().String(), "year_month": expense.PurchaseDate().YearMonth().String(),
				"is_edited": detail.Edited(), "created_at": timestamp, "updated_at": timestamp,
			})
			if err != nil {
				return fmt.Errorf("marshal new expense detail: %w", err)
			}
			items = append(items, ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{
				TableName: aws.String(r.ExpenseDetails.Name()), Item: item,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}})
			continue
		}
		updateExpression := "SET GSI1PK=:gpk,GSI1SK=:gsk,#name=:name,category=:category,category_source=:categorySource,amount=:amount,quantity=:quantity,#source=:source,store_name=:store,purchase_date=:date,year_month=:month,is_edited=:edited,updated_at=:updated,tax_mode=:taxMode,tax_allocation=:taxAllocation,tax_status=:taxStatus,tax_reason=:taxReason"
		remove := []string{}
		if detail.SuggestedTaxRate() != nil {
			values[":suggestedTaxRate"] = numberValue(*detail.SuggestedTaxRate())
			updateExpression += ",suggested_tax_rate=:suggestedTaxRate"
		} else {
			remove = append(remove, "suggested_tax_rate")
		}
		if detail.TaxIncludedAmount() != nil {
			values[":taxIncluded"] = numberValue(*detail.TaxIncludedAmount())
			updateExpression += ",tax_included_amount=:taxIncluded"
		} else {
			remove = append(remove, "tax_included_amount")
		}
		if detail.TaxRate() != nil {
			values[":taxRate"] = numberValue(*detail.TaxRate())
			updateExpression += ",tax_rate=:taxRate"
		} else {
			remove = append(remove, "tax_rate")
		}
		if len(remove) > 0 {
			updateExpression += " REMOVE " + strings.Join(remove, ",")
		}
		items = append(items, ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{
			TableName: aws.String(r.ExpenseDetails.Name()), Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(DetailPK(userID, expenseID)), "SK": stringValue(DetailSK(detail.ID().String()))},
			UpdateExpression:    aws.String(updateExpression),
			ConditionExpression: aws.String("attribute_exists(PK)"), ExpressionAttributeNames: map[string]string{"#name": "name", "#source": "source"}, ExpressionAttributeValues: values,
		}})
	}
	for _, detail := range previousDetails {
		if _, exists := current[detail.ID()]; exists {
			continue
		}
		items = append(items, ddbtypes.TransactWriteItem{Delete: &ddbtypes.Delete{
			TableName:           aws.String(r.ExpenseDetails.Name()),
			Key:                 map[string]ddbtypes.AttributeValue{"PK": stringValue(DetailPK(userID, expenseID)), "SK": stringValue(DetailSK(detail.ID().String()))},
			ConditionExpression: aws.String("attribute_exists(PK)"),
		}})
	}
	if len(items) > 100 {
		return application.ErrInvalidInput
	}
	_, err := r.Client.TransactWriteItems(ctx, &awssdk.TransactWriteItemsInput{TransactItems: items})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return application.ErrExpenseNotFound
	}
	return err
}

// restoreExpense は永続化された属性を値オブジェクトへ変換し、集約の不変条件を検証して復元する。
func restoreExpense(userID common.UserID, expenseID domain.ExpenseID, item expenseItem, details []domain.ExpenseDetail) (domain.Expense, error) {
	purchaseDate, err := common.NewPurchaseDate(item.PurchaseDate)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("restore expense %s: %w", expenseID, err)
	}
	readAmount, err := common.NewReadAmount(item.ReadAmount)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("restore expense %s: %w", expenseID, err)
	}
	// 手入力の支出は解析依頼を持たないので空を許容する
	sourceRequestID, _ := common.NewAnalysisRequestID(item.AnalysisRequestID)
	source, err := domain.NewRecordSource(item.Source)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("restore expense %s: %w", expenseID, err)
	}
	updatedAt, err := time.Parse(time.RFC3339, item.UpdatedAt)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("restore expense %s updated_at: %w", expenseID, err)
	}
	expense, err := domain.RestoreExpense(domain.ExpenseState{
		ID: expenseID, UserID: userID, SourceRequestID: sourceRequestID,
		StoreName: item.StoreName, PurchaseDate: purchaseDate, ReadAmount: readAmount, AnalysisEvidence: item.AnalysisEvidence,
		Adjustment: domain.NewAdjustmentAmount(item.AdjustmentAmount), Details: details, Edited: item.IsEdited, Source: source, UpdatedAt: updatedAt,
	})
	if err != nil {
		return domain.Expense{}, fmt.Errorf("restore expense %s: %w", expenseID, err)
	}
	return expense, nil
}

func restoreDetail(item expenseDetailItem) (domain.ExpenseDetail, error) {
	id, ok := domain.NewExpenseDetailID(item.DetailID)
	if !ok {
		return domain.ExpenseDetail{}, domain.ErrInvalidExpenseDetail
	}
	amount, err := common.NewDetailAmount(item.Amount)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	quantity, err := common.NewQuantity(item.Quantity)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	category, err := common.NewCategory(item.Category)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	source, err := domain.NewCategorySource(item.CategorySource)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	recordSource, err := domain.NewRecordSource(item.Source)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	detail, err := domain.RestoreExpenseDetail(id, item.Name, amount, quantity, category, source, recordSource, item.IsEdited)
	if err != nil {
		return domain.ExpenseDetail{}, err
	}
	if err := detail.SetTaxEvidence(item.TaxRate, item.TaxMode, item.TaxIncludedAmount, item.TaxAllocation); err != nil {
		return domain.ExpenseDetail{}, err
	}
	if err := detail.SetTaxInference(item.TaxStatus, item.TaxReason, item.SuggestedTaxRate); err != nil {
		return domain.ExpenseDetail{}, err
	}
	return detail, nil
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }
func numberValue(v int64) ddbtypes.AttributeValue {
	return &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", v)}
}
func boolValue(v bool) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberBOOL{Value: v} }
