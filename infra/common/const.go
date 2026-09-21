// Package common は stage に依存しない定数と、stage を付けて実際のリソース名を返す値オブジェクトを持つ。
package common

import "fmt"

const (
	ProjectName         = "snap_kakeibo"
	ProjectResourceName = "snap-kakeibo"
	AWSAccountID        = "654654388040"
	// 既存リソースとの整合のためソウルを使う
	AWSRegion = "ap-northeast-2"
	// UnsetParameterValue は手動投入が必要な SSM パラメータの初期値
	UnsetParameterValue = "UNSET"
)

// Stage はデプロイ先の環境。
type Stage string

const (
	StageDev Stage = "dev"
	StageStg Stage = "stg"
	StagePrd Stage = "prd"
)

// ResourceName は stage を除いたリソース名。For / Dev で実際の名前になる。
//
//	common.UsersTableName.Dev() // "dev-snap-kakeibo-users"
type ResourceName string

func (n ResourceName) For(stage Stage) string {
	return fmt.Sprintf("%s-%s-%s", stage, ProjectResourceName, n)
}

func (n ResourceName) Dev() string { return n.For(StageDev) }

// ParameterName は stage を除いた SSM パラメータ名。For / Dev で実際の名前になる。
//
//	common.OpenAIAPIKeyParameterName.Dev() // "/dev/snap-kakeibo/openai/api-key"
type ParameterName string

func (n ParameterName) For(stage Stage) string {
	return fmt.Sprintf("/%s/%s/%s", stage, ProjectResourceName, n)
}

func (n ParameterName) Dev() string { return n.For(StageDev) }

// スタック
const (
	StorageStackName  ResourceName = "storage"
	AppStackName      ResourceName = "app"
	PipelineStackName ResourceName = "pipeline"
)

// FunctionNames は Lambda の一覧。backend/cmd/{name} と 1:1 で、関数ごとに ECR リポジトリと
// デプロイ中のイメージタグを持つ SSM パラメータができる。関数を足すときはここにも追加する
var FunctionNames = []string{"hello", "auth-login", "auth-refresh", "auth-logout", "auth-check", "upload", "analyze-receipt", "retry-analysis", "list-analysis-requests", "get-expense"}

// ImageTagParameterName は関数のデプロイ中イメージタグを持つ SSM パラメータ名(stage 抜き)。
// image:push が更新し、App スタックが deploy 時に解決する
func ImageTagParameterName(functionName string) ParameterName {
	return ParameterName("functions/" + functionName + "/image-tag")
}

// LogGroupName は関数のロググループ名(stage 抜き)。Storage スタックが作り、App スタックの Lambda が書き込む
//
//	common.LogGroupName("hello").Dev() // "dev-snap-kakeibo-hello_lambda"
func LogGroupName(functionName string) ResourceName {
	return ResourceName(functionName + "_lambda")
}

// DynamoDB
const (
	UsersTableName            ResourceName = "users"
	RefreshTokensTableName    ResourceName = "refresh-tokens"
	MonthlySummariesTableName ResourceName = "monthly-summaries"
	AnalysisRequestsTableName ResourceName = "analysis-requests"
	ExpensesTableName         ResourceName = "expenses"
	ExpenseDetailsTableName   ResourceName = "expense-details"
)

// S3
const (
	ReceiptBucketName  ResourceName = "receipt"
	FrontendBucketName ResourceName = "front"
)

// GSI
const (
	RefreshTokenUserIndex     = "refresh_token_user_index"
	AnalysisRequestMonthIndex = "analysis_request_month_index"
	DetailMonthAmountIndex    = "detail_month_amount_index"
)

// S3 プレフィックス
const (
	ReceiptsPrefix        = "receipts/"
	AnalysisResultsPrefix = "analysis-results/"
)

// SQS
const (
	AnalyzeQueueName ResourceName = "analyze"
	AnalyzeDLQName   ResourceName = "analyze-dlq"
)

// SNS
const (
	AlertTopicName ResourceName = "alert"
)

// SSM Parameter Store。値は CDK で作らず手動投入する
const (
	PasswordPepperParameterName ParameterName = "auth/password-pepper"
	JWTSecretParameterName      ParameterName = "auth/jwt-secret"
	OpenAIAPIKeyParameterName   ParameterName = "openai/api-key"
)

// CI/CD 用 SSM Parameter Store。値は CDK で作らず ensure-parameters と pipeline が管理する
const (
	GitHubConnectionARNParameterName          ParameterName = "cicd/github-connection-arn"
	BackendLastSuccessfulCommitParameterName  ParameterName = "cicd/backend/last-successful-commit"
	FrontendLastSuccessfulCommitParameterName ParameterName = "cicd/frontend/last-successful-commit"
	InfraLastSuccessfulCommitParameterName    ParameterName = "cicd/infra/last-successful-commit"
)

// Lambda 環境変数名。backend の実装と揃える
const (
	// EnvStage は実行環境(dev / stg / prd)。backend の logger の environment と awsconfig の向き先に使う
	EnvStage                 = "STAGE"
	EnvUsersTable            = "USERS_TABLE"
	EnvRefreshTokensTable    = "REFRESH_TOKENS_TABLE"
	EnvMonthlySummariesTable = "MONTHLY_SUMMARIES_TABLE"
	EnvAnalysisRequestsTable = "ANALYSIS_REQUESTS_TABLE"
	EnvExpensesTable         = "EXPENSES_TABLE"
	EnvExpenseDetailsTable   = "EXPENSE_DETAILS_TABLE"
	EnvReceiptBucket         = "RECEIPT_BUCKET"
	EnvAnalyzeQueueURL       = "ANALYZE_QUEUE_URL"
	EnvImageMaxEdge          = "IMAGE_MAX_EDGE"
	EnvSSMPasswordPepper     = "SSM_PASSWORD_PEPPER"
	EnvSSMJWTSecret          = "SSM_JWT_SECRET"
	EnvSSMOpenAIAPIKey       = "SSM_OPENAI_API_KEY"
	EnvOpenAIModel           = "OPENAI_MODEL"
	EnvOpenAIReasoningEffort = "OPENAI_REASONING_EFFORT"
)
