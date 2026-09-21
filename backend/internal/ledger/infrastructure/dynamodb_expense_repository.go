// Package infrastructure は家計簿コンテキストの application が定義した interface の DynamoDB 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"

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
	ReadAmount        int64  `dynamodbav:"read_amount"`
	AdjustmentAmount  int64  `dynamodbav:"adjustment_amount"`
	IsEdited          bool   `dynamodbav:"is_edited"`
}

// expenseDetailItem は expense_details の項目のうち支出明細の復元に使う属性。
type expenseDetailItem struct {
	DetailID       string `dynamodbav:"detail_id"`
	Name           string `dynamodbav:"name"`
	Category       string `dynamodbav:"category"`
	CategorySource string `dynamodbav:"category_source"`
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
}

// DynamoDBExpenseRepository は expenses と expense_details から支出集約を復元する。
type DynamoDBExpenseRepository struct {
	Expenses       *libdynamodb.Table
	ExpenseDetails *libdynamodb.Table
}

var _ application.ExpenseFinder = DynamoDBExpenseRepository{}

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
	expense, err := domain.RestoreExpense(domain.ExpenseState{
		ID: expenseID, UserID: userID, SourceRequestID: sourceRequestID,
		StoreName: item.StoreName, PurchaseDate: purchaseDate, ReadAmount: readAmount,
		Adjustment: domain.NewAdjustmentAmount(item.AdjustmentAmount), Details: details, Edited: item.IsEdited,
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
	return domain.NewExpenseDetail(id, item.Name, amount, quantity, category, source)
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }
