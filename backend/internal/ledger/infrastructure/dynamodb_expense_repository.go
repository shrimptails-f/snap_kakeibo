// Package infrastructure は家計簿コンテキストの application が定義した interface の DynamoDB 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
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
	AdjustmentAmount  int64  `dynamodbav:"adjustment_amount"`
	IsEdited          bool   `dynamodbav:"is_edited"`
	Source            string `dynamodbav:"source"`
	UpdatedAt         string `dynamodbav:"updated_at"`
}

// expenseDetailItem は expense_details の項目のうち支出明細の復元に使う属性。
type expenseDetailItem struct {
	ExpenseID      string `dynamodbav:"expense_id"`
	DetailID       string `dynamodbav:"detail_id"`
	Name           string `dynamodbav:"name"`
	Category       string `dynamodbav:"category"`
	CategorySource string `dynamodbav:"category_source"`
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
	Source         string `dynamodbav:"source"`
	IsEdited       bool   `dynamodbav:"is_edited"`
	StoreName      string `dynamodbav:"store_name"`
	PurchaseDate   string `dynamodbav:"purchase_date"`
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
				Amount: record.Amount, Quantity: record.Quantity, Source: source, IsEdited: record.IsEdited,
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
func (r DynamoDBExpenseRepository) Save(ctx context.Context, expense domain.Expense, updatedAt time.Time) error {
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
	for _, detail := range expense.Details() {
		values := map[string]ddbtypes.AttributeValue{
			":gpk": stringValue(UserMonthPK(userID, expense.PurchaseDate().YearMonth().String())), ":gsk": stringValue(DetailMonthSK(detail.Amount().Yen(), expense.PurchaseDate().String(), detail.ID().String())),
			":name": stringValue(detail.Name()), ":category": stringValue(detail.Category().String()), ":categorySource": stringValue(detail.CategorySource().String()),
			":amount": numberValue(detail.Amount().Yen()), ":quantity": numberValue(detail.Quantity().Int64()), ":source": stringValue(detail.Source().String()),
			":store": stringValue(expense.StoreName()), ":date": stringValue(expense.PurchaseDate().String()), ":month": stringValue(expense.PurchaseDate().YearMonth().String()),
			":edited": boolValue(detail.Edited()), ":updated": stringValue(timestamp),
		}
		items = append(items, ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{
			TableName: aws.String(r.ExpenseDetails.Name()), Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(DetailPK(userID, expenseID)), "SK": stringValue(DetailSK(detail.ID().String()))},
			UpdateExpression:    aws.String("SET GSI1PK=:gpk,GSI1SK=:gsk,#name=:name,category=:category,category_source=:categorySource,amount=:amount,quantity=:quantity,#source=:source,store_name=:store,purchase_date=:date,year_month=:month,is_edited=:edited,updated_at=:updated"),
			ConditionExpression: aws.String("attribute_exists(PK)"), ExpressionAttributeNames: map[string]string{"#name": "name", "#source": "source"}, ExpressionAttributeValues: values,
		}})
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
		StoreName: item.StoreName, PurchaseDate: purchaseDate, ReadAmount: readAmount,
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
	return domain.RestoreExpenseDetail(id, item.Name, amount, quantity, category, source, recordSource, item.IsEdited)
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }
func numberValue(v int64) ddbtypes.AttributeValue {
	return &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", v)}
}
func boolValue(v bool) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberBOOL{Value: v} }
