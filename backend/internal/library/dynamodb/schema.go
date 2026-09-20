package dynamodb

import (
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// テーブルの基本名(stage / prefix 抜き)。infra/common/const.go の *TableName と揃える。
const (
	UsersTableName            = "users"
	RefreshTokensTableName    = "refresh-tokens"
	MonthlySummariesTableName = "monthly-summaries"
	UploadHistoriesTableName  = "upload-histories"
	BillingsTableName         = "billings"
	BillingDetailsTableName   = "billing-details"
)

// GSI 名。infra/common/const.go と揃える。
const (
	RefreshTokenUserIndex  = "refresh_token_user_index"
	UploadMonthIndex       = "upload_month_index"
	DetailMonthAmountIndex = "detail_month_amount_index"
)

// Schema はテーブルのキー構成。infra/stacks/storage.go の newTable / addGSI1 と同じ形を backend 側で持ち、
// ローカル(Floci)にテーブルを作るときに使う。属性はすべて文字列キー。
type Schema struct {
	// Name は基本名(users など)。実際のテーブル名は prefix を付けて決める
	Name string
	// PartitionKey は PK の属性名
	PartitionKey string
	// SortKey は SK の属性名。空ならソートキーなし
	SortKey string
	// GSIs は GSI の一覧
	GSIs []GSI
}

// GSI は Global Secondary Index のキー構成。射影は ALL 固定。
type GSI struct {
	Name         string
	PartitionKey string
	SortKey      string
}

// 本番と同じキー構成のスキーマ。infra/stacks/storage.go を変更したらここも合わせる。
var (
	UsersSchema            = Schema{Name: UsersTableName, PartitionKey: "PK"}
	RefreshTokensSchema    = Schema{Name: RefreshTokensTableName, PartitionKey: "PK", GSIs: []GSI{gsi1(RefreshTokenUserIndex)}}
	MonthlySummariesSchema = Schema{Name: MonthlySummariesTableName, PartitionKey: "PK", SortKey: "SK"}
	UploadHistoriesSchema  = Schema{Name: UploadHistoriesTableName, PartitionKey: "PK", SortKey: "SK", GSIs: []GSI{gsi1(UploadMonthIndex)}}
	BillingsSchema         = Schema{Name: BillingsTableName, PartitionKey: "PK", SortKey: "SK"}
	BillingDetailsSchema   = Schema{Name: BillingDetailsTableName, PartitionKey: "PK", SortKey: "SK", GSIs: []GSI{gsi1(DetailMonthAmountIndex)}}
)

// AllSchemas は本番に存在するテーブルのスキーマをすべて返す。
func AllSchemas() []Schema {
	return []Schema{UsersSchema, RefreshTokensSchema, MonthlySummariesSchema, UploadHistoriesSchema, BillingsSchema, BillingDetailsSchema}
}

func gsi1(name string) GSI { return GSI{Name: name, PartitionKey: "GSI1PK", SortKey: "GSI1SK"} }

// Validate はスキーマが CreateTable に渡せる形か確認する。
func (s Schema) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("dynamodb: schema name is empty")
	}
	if strings.TrimSpace(s.PartitionKey) == "" {
		return fmt.Errorf("dynamodb: schema %s has no partition key", s.Name)
	}
	for _, g := range s.GSIs {
		if strings.TrimSpace(g.Name) == "" || strings.TrimSpace(g.PartitionKey) == "" {
			return fmt.Errorf("dynamodb: schema %s has a GSI without name or partition key", s.Name)
		}
	}
	return nil
}

// CreateTableInput は tableName でこのスキーマのテーブルを作る入力を返す。オンデマンド課金。
func (s Schema) CreateTableInput(tableName string) (*awssdk.CreateTableInput, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// 同じ属性が複数のキーに使われても AttributeDefinitions には 1 回だけ載せる
	attrs := map[string]struct{}{}
	var definitions []types.AttributeDefinition
	define := func(name string) {
		if name == "" {
			return
		}
		if _, ok := attrs[name]; ok {
			return
		}
		attrs[name] = struct{}{}
		definitions = append(definitions, types.AttributeDefinition{AttributeName: &name, AttributeType: types.ScalarAttributeTypeS})
	}

	in := &awssdk.CreateTableInput{
		TableName:   &tableName,
		BillingMode: types.BillingModePayPerRequest,
		KeySchema:   keySchema(s.PartitionKey, s.SortKey),
	}
	define(s.PartitionKey)
	define(s.SortKey)
	for _, g := range s.GSIs {
		in.GlobalSecondaryIndexes = append(in.GlobalSecondaryIndexes, types.GlobalSecondaryIndex{
			IndexName:  &g.Name,
			KeySchema:  keySchema(g.PartitionKey, g.SortKey),
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		})
		define(g.PartitionKey)
		define(g.SortKey)
	}
	in.AttributeDefinitions = definitions
	return in, nil
}

func keySchema(pk, sk string) []types.KeySchemaElement {
	schema := []types.KeySchemaElement{{AttributeName: &pk, KeyType: types.KeyTypeHash}}
	if sk != "" {
		schema = append(schema, types.KeySchemaElement{AttributeName: &sk, KeyType: types.KeyTypeRange})
	}
	return schema
}
