// Package localenv は Floci 上に作るローカル開発用リソースの名前と、Lambda に渡す環境変数を 1 か所で決める。
//
// tools/localseed がここで決めた名前でリソースを作り、tools/localapi が同じ名前を各 Lambda の環境変数として渡す。
// 本番の名前(infra/common)と揃える必要はないが、テーブルの基本名は internal/library/dynamodb の Schema と揃える。
package localenv

import (
	"fmt"
	"os"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/stage"
)

// 環境変数名。infra/common/const.go の Env* と同じ値。
const (
	EnvStage                 = "STAGE"
	EnvEndpointURL           = "AWS_ENDPOINT_URL"
	EnvRegion                = "AWS_REGION"
	EnvUsersTable            = "USERS_TABLE"
	EnvRefreshTokensTable    = "REFRESH_TOKENS_TABLE"
	EnvMonthlySummariesTable = "MONTHLY_SUMMARIES_TABLE"
	EnvAnalysisRequestsTable = "ANALYSIS_REQUESTS_TABLE"
	EnvExpensesTable         = "EXPENSES_TABLE"
	EnvExpenseDetailsTable   = "EXPENSE_DETAILS_TABLE"
	EnvReceiptBucket         = "RECEIPT_BUCKET"
	EnvAnalyzeQueueURL       = "ANALYZE_QUEUE_URL"
	EnvSSMJWTSecret          = "SSM_JWT_SECRET"
	EnvLogLevel              = "LOG_LEVEL"
)

// 既定値。devcontainer(.devcontainer/compose.yaml)の Floci に合わせる。
const (
	DefaultEndpointURL = "http://floci:4566"
	DefaultRegion      = "ap-northeast-2"
)

// Floci 上のリソース名。テストの一時リソース(test-* / ランダム名)と混ざらないよう local- を付ける。
const (
	prefix             = "local"
	ReceiptBucket      = prefix + "-snap-kakeibo-receipt"
	AnalyzeQueueName   = prefix + "-snap-kakeibo-analyze"
	JWTSecretParameter = "/" + prefix + "/auth/jwt-secret"
)

// TableName は schema の基本名に local- を付けたテーブル名を返す。
func TableName(schema libdynamodb.Schema) string {
	return prefix + "-" + schema.Name
}

// LambdaEnvironment は各 Lambda に渡す環境変数。analyzeQueueURL は Floci が採番するので seed 後に引く。
func LambdaEnvironment(endpointURL, region, analyzeQueueURL string) map[string]string {
	return map[string]string{
		EnvStage:                 stage.Local.String(),
		EnvEndpointURL:           endpointURL,
		EnvRegion:                region,
		EnvUsersTable:            TableName(libdynamodb.UsersSchema),
		EnvRefreshTokensTable:    TableName(libdynamodb.RefreshTokensSchema),
		EnvMonthlySummariesTable: TableName(libdynamodb.MonthlySummariesSchema),
		EnvAnalysisRequestsTable: TableName(libdynamodb.AnalysisRequestsSchema),
		EnvExpensesTable:         TableName(libdynamodb.ExpensesSchema),
		EnvExpenseDetailsTable:   TableName(libdynamodb.ExpenseDetailsSchema),
		EnvReceiptBucket:         ReceiptBucket,
		EnvAnalyzeQueueURL:       analyzeQueueURL,
		EnvSSMJWTSecret:          JWTSecretParameter,
	}
}

// EnsureLocalProcessEnv は自プロセスの STAGE / AWS_ENDPOINT_URL / AWS_REGION を Floci 向けに整える。
// awsconfig.Load はこれらを環境変数から読むため、ツール起動時に一度だけ呼ぶ。
// STAGE が local / ci 以外に設定されていれば、本物の AWS に向いてしまうのでエラーにする。
func EnsureLocalProcessEnv() error {
	if raw := os.Getenv(EnvStage); raw != "" {
		st, err := stage.Parse(raw)
		if err != nil {
			return err
		}
		if !st.IsLocal() {
			return fmt.Errorf("localenv: %s=%q is not a local stage; refusing to touch a real AWS account", EnvStage, raw)
		}
	} else if err := os.Setenv(EnvStage, stage.Local.String()); err != nil {
		return err
	}
	for key, value := range map[string]string{EnvEndpointURL: DefaultEndpointURL, EnvRegion: DefaultRegion} {
		if os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
