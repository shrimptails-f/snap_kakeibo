package stacks

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3notifications"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssnssubscriptions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

// StorageStackProps は Storage スタックの入力。
type StorageStackProps struct {
	awscdk.StackProps
	Config config.Config
}

// StorageStack は失うと困るリソースをまとめる。
// DynamoDB / S3 / ECR のデータ、Lambda のログ、DLQ に溜まった失敗メッセージ、
// 確認済みのメール購読が該当する。App スタックからのみ参照され、逆方向の依存は持たない。
type StorageStack struct {
	awscdk.Stack

	// Repositories は関数名 -> Lambda イメージ用 ECR
	Repositories map[string]awsecr.Repository
	// LogGroups は関数名 -> Lambda のロググループ。App スタックを destroy してもログを残すためここに置く
	LogGroups map[string]awslogs.LogGroup
	// Bucket はレシート画像と OpenAI 生レスポンス JSON
	Bucket awss3.Bucket
	// FrontendBucket は React のビルド成果物。人間が push し、App スタックの CloudFront が配信する
	FrontendBucket awss3.Bucket

	UsersTable            awsdynamodb.Table
	RefreshTokensTable    awsdynamodb.Table
	MonthlySummariesTable awsdynamodb.Table
	AnalysisRequestsTable awsdynamodb.Table
	ExpensesTable         awsdynamodb.Table
	ExpenseDetailsTable   awsdynamodb.Table

	AnalyzeQueue awssqs.Queue

	AlertTopic awssns.Topic
}

func NewStorageStack(scope constructs.Construct, id string, props *StorageStackProps) *StorageStack {
	stack := awscdk.NewStack(scope, jsii.String(id), &props.StackProps)
	cfg := props.Config
	s := &StorageStack{Stack: stack}

	s.Repositories = newRepositories(stack, cfg)
	s.LogGroups = newLogGroups(stack, cfg)
	s.Bucket = newReceiptBucket(stack, cfg)
	s.FrontendBucket = newFrontendBucket(stack, cfg)

	// users にはユーザー本体とログイン試行カウンタを置く。TTL はカウンタの expires_at(Unix 秒)を対象にし、
	// ユーザーアイテムは expires_at を持たないので消えない。
	s.UsersTable = newTable(stack, "UsersTable", cfg, cfg.Tables.Users, false, jsii.String("expires_at"))
	// refresh token はユーザーとは更新単位が異なるため専用テーブルに置き、期限切れは TTL で自動削除する。
	// GSI でユーザー単位の一覧・一括失効(全端末ログアウト)ができるようにする。
	s.RefreshTokensTable = newTable(stack, "RefreshTokensTable", cfg, cfg.Tables.RefreshTokens, false, jsii.String("expires_at"))
	addGSI1(s.RefreshTokensTable, common.RefreshTokenUserIndex)
	s.MonthlySummariesTable = newTable(stack, "MonthlySummariesTable", cfg, cfg.Tables.MonthlySummaries, true, nil)
	s.AnalysisRequestsTable = newTable(stack, "AnalysisRequestsTable", cfg, cfg.Tables.AnalysisRequests, true, nil)
	s.ExpensesTable = newTable(stack, "ExpensesTable", cfg, cfg.Tables.Expenses, true, nil)
	s.ExpenseDetailsTable = newTable(stack, "ExpenseDetailsTable", cfg, cfg.Tables.ExpenseDetails, true, nil)
	addGSI1(s.AnalysisRequestsTable, common.AnalysisRequestMonthIndex)
	addGSI1(s.ExpenseDetailsTable, common.DetailMonthAmountIndex)

	s.AlertTopic = awssns.NewTopic(stack, jsii.String("AlertTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(cfg.Topics.Alert),
		DisplayName: jsii.String(common.ProjectResourceName + " alert"),
	})
	if cfg.AlertEmail != "" {
		s.AlertTopic.AddSubscription(awssnssubscriptions.NewEmailSubscription(jsii.String(cfg.AlertEmail), nil))
	} else {
		awscdk.Annotations_Of(stack).AddWarning(jsii.String("AlertEmail is empty in config; DLQ alarms will not notify anyone"))
	}

	s.AnalyzeQueue = newQueueWithDLQ(stack, "Analyze", cfg.Queues.Analyze, cfg.Queues.AnalyzeDLQ, cfg.Timeouts.Analyze, cfg.MaxReceiveCount, s.AlertTopic)

	// S3 ObjectCreated -> Analyze Queue。バケットとキューが同じスタックにあるので循環参照にならない
	s.Bucket.AddEventNotification(
		awss3.EventType_OBJECT_CREATED,
		awss3notifications.NewSqsDestination(s.AnalyzeQueue),
		&awss3.NotificationKeyFilter{Prefix: jsii.String(common.ReceiptsPrefix)},
	)

	awscdk.NewCfnOutput(stack, jsii.String("ReceiptBucketName"), &awscdk.CfnOutputProps{Value: s.Bucket.BucketName()})
	awscdk.NewCfnOutput(stack, jsii.String("FrontendBucketName"), &awscdk.CfnOutputProps{Value: s.FrontendBucket.BucketName()})
	awscdk.NewCfnOutput(stack, jsii.String("AnalyzeQueueUrl"), &awscdk.CfnOutputProps{Value: s.AnalyzeQueue.QueueUrl()})

	return s
}

// newRepositories は関数ごとに Lambda イメージ用の ECR を作る。
// タグを IMMUTABLE にすると同じタグの差し替えによる「CDK が変更を検知しない」事故を防げる。
func newRepositories(scope constructs.Construct, cfg config.Config) map[string]awsecr.Repository {
	repos := make(map[string]awsecr.Repository, len(cfg.Functions))
	for _, f := range cfg.Functions {
		id := constructID(f.Name)
		repo := awsecr.NewRepository(scope, jsii.String(id+"Repository"), &awsecr.RepositoryProps{
			RepositoryName:     jsii.String(f.Repository.Name),
			ImageTagMutability: f.Repository.ImageTagMutability,
			RemovalPolicy:      cfg.RemovalPolicy,
			EmptyOnDelete:      jsii.Bool(!cfg.Retain()),
			LifecycleRules: &[]*awsecr.LifecycleRule{{
				Description:   jsii.String(fmt.Sprintf("keep the latest %d images", int(f.Repository.MaxImageCount))),
				MaxImageCount: jsii.Number(f.Repository.MaxImageCount),
			}},
		})
		// App スタックの Lambda がイメージを pull できるようにする。
		// App 側で付けると Storage スタックのテンプレートが App の deploy で変わるため、ここで付ける
		repo.GrantPull(awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil))
		awscdk.NewCfnOutput(scope, jsii.String(id+"RepositoryUri"), &awscdk.CfnOutputProps{Value: repo.RepositoryUri()})
		repos[f.Name] = repo
	}
	return repos
}

// newLogGroups は関数ごとに Lambda のロググループを作る。
// Lambda 側で LogGroup を明示するので /aws/lambda/ 以外の名前でも Lambda が書き込める。
func newLogGroups(scope constructs.Construct, cfg config.Config) map[string]awslogs.LogGroup {
	groups := make(map[string]awslogs.LogGroup, len(cfg.Functions))
	for _, f := range cfg.Functions {
		groups[f.Name] = awslogs.NewLogGroup(scope, jsii.String(constructID(f.Name)+"LogGroup"), &awslogs.LogGroupProps{
			LogGroupName:  jsii.String(f.LogGroup),
			Retention:     awslogs.RetentionDays_ONE_MONTH,
			RemovalPolicy: cfg.RemovalPolicy,
		})
	}
	return groups
}

// constructID は "analyze-receipt" のような関数名を "AnalyzeReceipt" に変えて construct ID に使う。
func constructID(name string) string {
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '-' || r == '_' {
			upper = true
			continue
		}
		if upper {
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// newReceiptBucket はレシート画像と OpenAI 生レスポンス JSON を置くバケットを作る。
// ブラウザから Presigned PUT する前提なので CORS を許可する。
func newReceiptBucket(scope constructs.Construct, cfg config.Config) awss3.Bucket {
	return awss3.NewBucket(scope, jsii.String("ReceiptBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(cfg.Buckets.Receipts),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		EnforceSSL:        jsii.Bool(true),
		RemovalPolicy:     cfg.RemovalPolicy,
		AutoDeleteObjects: jsii.Bool(!cfg.Retain()),
		LifecycleRules: &[]*awss3.LifecycleRule{{
			Id: jsii.String("ExpireAnalysisResults"), Enabled: jsii.Bool(true), Prefix: jsii.String(common.AnalysisResultsPrefix), Expiration: awscdk.Duration_Days(jsii.Number(90)),
		}},
		Cors: &[]*awss3.CorsRule{{
			AllowedOrigins: jsii.Strings(cfg.CORSAllowedOrigins...),
			AllowedMethods: &[]awss3.HttpMethods{awss3.HttpMethods_PUT},
			AllowedHeaders: jsii.Strings("*"),
			MaxAge:         jsii.Number(3000),
		}},
	})
}

// newFrontendBucket は React のビルド成果物を置くバケットを作る。
// 配信は App スタックの CloudFront(OAC) が行う。Distribution の ARN は App 側で決まり
// ここから参照すると循環になるため、このアカウントの Distribution 全体に読み取りを許可する。
func newFrontendBucket(scope constructs.Construct, cfg config.Config) awss3.Bucket {
	bucket := awss3.NewBucket(scope, jsii.String("FrontendBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(cfg.Buckets.Frontend),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		EnforceSSL:        jsii.Bool(true),
		RemovalPolicy:     cfg.RemovalPolicy,
		AutoDeleteObjects: jsii.Bool(!cfg.Retain()),
	})
	bucket.AddToResourcePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:        jsii.String("AllowCloudFrontOAC"),
		Actions:    jsii.Strings("s3:GetObject"),
		Resources:  jsii.Strings(*bucket.ArnForObjects(jsii.String("*"))),
		Principals: &[]awsiam.IPrincipal{awsiam.NewServicePrincipal(jsii.String("cloudfront.amazonaws.com"), nil)},
		Conditions: &map[string]any{
			"ArnLike": map[string]any{
				"AWS:SourceArn": fmt.Sprintf("arn:aws:cloudfront::%s:distribution/*", cfg.AccountID),
			},
		},
	}))
	return bucket
}

// newTable は PK(+SK) の文字列キーを持つオンデマンドテーブルを作る。
func newTable(scope constructs.Construct, id string, cfg config.Config, name string, withSortKey bool, timeToLiveAttribute *string) awsdynamodb.Table {
	props := &awsdynamodb.TableProps{
		TableName:          jsii.String(name),
		PartitionKey:       &awsdynamodb.Attribute{Name: jsii.String("PK"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:        awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy:      cfg.RemovalPolicy,
		DeletionProtection: jsii.Bool(cfg.Retain()),
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(true),
		},
		TimeToLiveAttribute: timeToLiveAttribute,
	}
	if withSortKey {
		props.SortKey = &awsdynamodb.Attribute{Name: jsii.String("SK"), Type: awsdynamodb.AttributeType_STRING}
	}
	return awsdynamodb.NewTable(scope, jsii.String(id), props)
}

// addGSI1 は GSI1PK / GSI1SK をキーにする GSI を追加する。
func addGSI1(table awsdynamodb.Table, indexName string) {
	table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName:      jsii.String(indexName),
		PartitionKey:   &awsdynamodb.Attribute{Name: jsii.String("GSI1PK"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:        &awsdynamodb.Attribute{Name: jsii.String("GSI1SK"), Type: awsdynamodb.AttributeType_STRING},
		ProjectionType: awsdynamodb.ProjectionType_ALL,
	})
}

// newQueueWithDLQ は標準キューと DLQ、DLQ 監視アラームをまとめて作る。
// 可視性タイムアウトは消費側 Lambda のタイムアウトの6倍にする。
func newQueueWithDLQ(scope constructs.Construct, id, name, dlqName string, lambdaTimeout awscdk.Duration, maxReceiveCount float64, alertTopic awssns.ITopic) awssqs.Queue {
	dlq := awssqs.NewQueue(scope, jsii.String(id+"DLQ"), &awssqs.QueueProps{
		QueueName:       jsii.String(dlqName),
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		EnforceSSL:      jsii.Bool(true),
	})
	queue := awssqs.NewQueue(scope, jsii.String(id+"Queue"), &awssqs.QueueProps{
		QueueName:         jsii.String(name),
		VisibilityTimeout: visibilityTimeoutFor(lambdaTimeout),
		EnforceSSL:        jsii.Bool(true),
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			Queue:           dlq,
			MaxReceiveCount: jsii.Number(maxReceiveCount),
		},
	})

	alarm := dlq.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
		Period:    awscdk.Duration_Minutes(jsii.Number(1)),
		Statistic: jsii.String("Maximum"),
	}).CreateAlarm(scope, jsii.String(id+"DLQAlarm"), &awscloudwatch.CreateAlarmOptions{
		AlarmName:          jsii.String(dlqName + "-has-messages"),
		AlarmDescription:   jsii.String("Messages arrived in " + dlqName),
		Threshold:          jsii.Number(0),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	alarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))

	return queue
}

// visibilityTimeoutFor は Lambda タイムアウトから SQS 可視性タイムアウトを求める(AWS推奨の6倍)。
func visibilityTimeoutFor(lambdaTimeout awscdk.Duration) awscdk.Duration {
	return awscdk.Duration_Seconds(jsii.Number(*lambdaTimeout.ToSeconds(nil) * 6))
}
