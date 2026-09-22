package config

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
)

// Dev は dev stage の設定。
func Dev() Config {
	return Config{
		Stage:     common.StageDev,
		AccountID: common.AWSAccountID,
		Region:    common.AWSRegion,
		// TODO: DLQ アラートを受け取るメールアドレスを設定する
		AlertEmail: "",
		// TODO: フロントエンドの配信ドメインが決まったら絞る
		CORSAllowedOrigins: []string{"*"},
		// dev は作り直せればよいので残さない
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,

		StorageStackName:  common.StorageStackName.Dev(),
		AppStackName:      common.AppStackName.Dev(),
		PipelineStackName: common.PipelineStackName.Dev(),
		// 関数ごとに ECR を持つ。10 世代残し、同じタグの再 push はエラー
		Functions: functions(common.StageDev, ECRConfig{
			MaxImageCount:      10,
			ImageTagMutability: awsecr.TagMutability_IMMUTABLE,
		}),
		Buckets: Buckets{
			Receipts: common.ReceiptBucketName.Dev(),
			Frontend: common.FrontendBucketName.Dev(),
		},
		Tables: Tables{
			Users:            common.UsersTableName.Dev(),
			RefreshTokens:    common.RefreshTokensTableName.Dev(),
			MonthlySummaries: common.MonthlySummariesTableName.Dev(),
			AnalysisRequests: common.AnalysisRequestsTableName.Dev(),
			Expenses:         common.ExpensesTableName.Dev(),
			ExpenseDetails:   common.ExpenseDetailsTableName.Dev(),
		},
		Queues: Queues{
			Analyze:    common.AnalyzeQueueName.Dev(),
			AnalyzeDLQ: common.AnalyzeDLQName.Dev(),
		},
		Topics: Topics{
			Alert: common.AlertTopicName.Dev(),
		},
		Parameters: Parameters{
			PasswordPepper: common.PasswordPepperParameterName.Dev(),
			JWTSecret:      common.JWTSecretParameterName.Dev(),
			OpenAIAPIKey:   common.OpenAIAPIKeyParameterName.Dev(),
		},
		CI: CICDConfig{
			GitHubOwner: "shrimptails-f",
			GitHubRepo:  "snap_kakeibo",
			Branch:      "deploy/dev",
			Parameters: CICDParameters{
				GitHubConnectionARN:          common.GitHubConnectionARNParameterName.Dev(),
				BackendLastSuccessfulCommit:  common.BackendLastSuccessfulCommitParameterName.Dev(),
				FrontendLastSuccessfulCommit: common.FrontendLastSuccessfulCommitParameterName.Dev(),
				InfraLastSuccessfulCommit:    common.InfraLastSuccessfulCommitParameterName.Dev(),
			},
		},
		OpenAI: OpenAIConfig{Model: "gpt-5.6-terra", ReasoningEffort: "medium"},
		Timeouts: Timeouts{
			Analyze: awscdk.Duration_Minutes(jsii.Number(3)),
			API:     awscdk.Duration_Seconds(jsii.Number(29)),
		},
		Deployment: DeploymentConfig{
			Strategy: DeploymentStrategyAllAtOnce,
		},
		MaxReceiveCount: 3,
	}
}
