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

		StorageStackName: common.StorageStackName.Dev(),
		AppStackName:     common.AppStackName.Dev(),
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
			MonthlySummaries: common.MonthlySummariesTableName.Dev(),
			UploadHistories:  common.UploadHistoriesTableName.Dev(),
			Billings:         common.BillingsTableName.Dev(),
			BillingDetails:   common.BillingDetailsTableName.Dev(),
		},
		Queues: Queues{
			StartTextract:    common.StartTextractQueueName.Dev(),
			StartTextractDLQ: common.StartTextractDLQName.Dev(),
			ResultHandler:    common.ResultHandlerQueueName.Dev(),
			ResultHandlerDLQ: common.ResultHandlerDLQName.Dev(),
		},
		Topics: Topics{
			TextractCompletion: common.TextractCompletionTopicName.Dev(),
			Alert:              common.AlertTopicName.Dev(),
		},
		Parameters: Parameters{
			PasswordPepper: common.PasswordPepperParameterName.Dev(),
			JWTSecret:      common.JWTSecretParameterName.Dev(),
			OpenAIAPIKey:   common.OpenAIAPIKeyParameterName.Dev(),
			OpenAIModel:    common.OpenAIModelParameterName.Dev(),
		},
		Timeouts: Timeouts{
			StartTextract: awscdk.Duration_Seconds(jsii.Number(60)),
			ResultHandler: awscdk.Duration_Minutes(jsii.Number(15)),
			API:           awscdk.Duration_Seconds(jsii.Number(29)),
		},
		MaxReceiveCount: 3,
	}
}
