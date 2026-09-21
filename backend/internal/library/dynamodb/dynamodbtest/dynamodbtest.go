// Package dynamodbtest はローカル(Floci)の DynamoDB にテストごとの一時テーブルを作るヘルパーを提供する。
//
// テーブル名は毎回ランダムに決まり、テスト終了時に t.Cleanup で削除される。並列に走るテストや
// シナリオテスト(複数 API をまたぐ結合テスト)が互いのデータを見ないようにするため、
// 共有の固定名テーブルは作らない。
//
//	func TestLogin(t *testing.T) {
//		env := dynamodbtest.Connect(t)                    // STAGE が local / ci でなければ Skip
//		users := env.CreateTable(t, libdynamodb.UsersSchema) // "test-users-<random>" を作り、終了時に消す
//		repo := infrastructure.DynamoDBUserRepository{Table: users}
//		...
//	}
//
// シナリオテストで全テーブルが必要なら CreateAllTables を使う。
//
//	tables := env.CreateAllTables(t, "scenario")
//	tables.Users.Name() // "scenario-users-<random>"
//
// Lambda のバイナリに testing パッケージを混ぜないため、dynamodb 本体とは別パッケージにしている。
package dynamodbtest

import (
	"context"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/awsconfig"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/stage"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// DefaultPrefix は CreateTable に prefix を渡さないときのテーブル名の接頭辞。
const DefaultPrefix = "test"

// cleanupTimeout は t.Cleanup でテーブルを消すときの上限。テストの ctx は既に終わっているので別に持つ。
const cleanupTimeout = 30 * time.Second

// Env はローカル DynamoDB への接続一式。
type Env struct {
	// Config は awsconfig.Load が返した Floci 向けの設定。S3 / SQS など他のクライアントにも使える
	Config aws.Config
	// Client は業務コードに渡す読み書き用クライアント
	Client *libdynamodb.Client
	// Manager はテーブルの作成・削除
	Manager *libdynamodb.Manager
}

// Tables は本番と同じ構成の一時テーブル一式。
type Tables struct {
	Users            *libdynamodb.Table
	RefreshTokens    *libdynamodb.Table
	MonthlySummaries *libdynamodb.Table
	AnalysisRequests *libdynamodb.Table
	Expenses         *libdynamodb.Table
	ExpenseDetails   *libdynamodb.Table
}

// Connect は STAGE が local / ci のときローカル DynamoDB に繋いだ Env を返し、それ以外なら t.Skip する。
func Connect(t testing.TB) *Env {
	t.Helper()
	osw := oswrapper.New()
	st, err := stage.FromEnv(osw)
	if err != nil || !st.IsLocal() {
		t.Skipf("STAGE is not local / ci (stage=%q err=%v); skipping DynamoDB integration test", st, err)
	}
	cfg, err := awsconfig.Load(context.Background(), osw)
	if err != nil {
		t.Fatalf("dynamodbtest: load aws config: %v", err)
	}
	return &Env{Config: cfg, Client: libdynamodb.New(cfg, nil), Manager: libdynamodb.NewManager(cfg)}
}

// CreateTable はランダムな名前(DefaultPrefix + schema.Name + 乱数)でテーブルを作り、テスト終了時に削除する。
func (e *Env) CreateTable(t testing.TB, schema libdynamodb.Schema) *libdynamodb.Table {
	t.Helper()
	return e.CreateTableWithPrefix(t, DefaultPrefix, schema)
}

// CreateTableWithPrefix は prefix を指定してテーブルを作る。名前は prefix-<schema.Name>-<乱数>。
func (e *Env) CreateTableWithPrefix(t testing.TB, prefix string, schema libdynamodb.Schema) *libdynamodb.Table {
	t.Helper()
	name := libdynamodb.RandomTableName(prefix, schema)
	ctx, cancel := context.WithTimeout(context.Background(), libdynamodb.DefaultWaitTimeout)
	defer cancel()
	cleanup, err := e.Manager.CreateTable(ctx, name, schema)
	if err != nil {
		t.Fatalf("dynamodbtest: create table %s against %s: %v", name, aws.ToString(e.Config.BaseEndpoint), err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := cleanup(ctx); err != nil {
			// 残ったテーブルは次のテストの邪魔にはならない(名前が違う)が、Floci に溜まるので知らせる
			t.Errorf("dynamodbtest: delete table %s: %v", name, err)
		}
	})
	return e.Client.Table(name)
}

// CreateAllTables は本番と同じ 6 テーブルを prefix 付きのランダムな名前で作り、テスト終了時にまとめて削除する。
func (e *Env) CreateAllTables(t testing.TB, prefix string) Tables {
	t.Helper()
	return Tables{
		Users:            e.CreateTableWithPrefix(t, prefix, libdynamodb.UsersSchema),
		RefreshTokens:    e.CreateTableWithPrefix(t, prefix, libdynamodb.RefreshTokensSchema),
		MonthlySummaries: e.CreateTableWithPrefix(t, prefix, libdynamodb.MonthlySummariesSchema),
		AnalysisRequests: e.CreateTableWithPrefix(t, prefix, libdynamodb.AnalysisRequestsSchema),
		Expenses:         e.CreateTableWithPrefix(t, prefix, libdynamodb.ExpensesSchema),
		ExpenseDetails:   e.CreateTableWithPrefix(t, prefix, libdynamodb.ExpenseDetailsSchema),
	}
}
