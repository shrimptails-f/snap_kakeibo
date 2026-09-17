package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

// AppStackProps は App スタックの入力。
type AppStackProps struct {
	awscdk.StackProps
	Config  config.Config
	Storage *StorageStack
}

// AppStack は destroy して作り直せるリソースをまとめる。
// Lambda / API Gateway / イベントソースマッピング / CloudFront が該当する。
// Lambda は各ステップ(認証、アップロード、...)の実装と同時に newFunction で追加する。
type AppStack struct {
	awscdk.Stack

	// Distribution は React を配信し、/api/* を API へ転送する CloudFront
	Distribution awscloudfront.Distribution
	// API は Lambda を束ねる HTTP API
	API awsapigatewayv2.HttpApi

	cfg     config.Config
	storage *StorageStack
}

func NewAppStack(scope constructs.Construct, id string, props *AppStackProps) *AppStack {
	stack := awscdk.NewStack(scope, jsii.String(id), &props.StackProps)
	s := &AppStack{
		Stack:   stack,
		cfg:     props.Config,
		storage: props.Storage,
	}

	s.Distribution = newFrontendDistribution(stack, props.Config)
	s.API = newHTTPAPI(stack, props.Config)
	attachAPIToDistribution(s.Distribution, s.API)

	// 疎通確認。各ステップの Lambda はこの形で追加していく
	hello := s.newFunction("hello", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/hello", hello)

	upload := s.newFunction("upload", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/uploads", upload)
	s.storage.UploadHistoriesTable.GrantWriteData(upload)
	s.storage.Bucket.GrantPut(upload, jsii.String("receipts/*"))

	analyzeReceipt := s.newFunction("analyze-receipt", functionProps{MemorySize: 1024, Timeout: props.Config.Timeouts.Analyze})
	analyzeReceipt.AddEventSource(awslambdaeventsources.NewSqsEventSource(s.storage.AnalyzeQueue, &awslambdaeventsources.SqsEventSourceProps{
		BatchSize:      jsii.Number(1),
		MaxConcurrency: jsii.Number(5),
	}))
	s.storage.Bucket.GrantRead(analyzeReceipt, jsii.String("receipts/*"))
	s.storage.Bucket.GrantPut(analyzeReceipt, jsii.String("analysis-results/*"))
	s.storage.UploadHistoriesTable.GrantReadWriteData(analyzeReceipt)
	s.storage.BillingsTable.GrantReadWriteData(analyzeReceipt)
	s.storage.BillingDetailsTable.GrantReadWriteData(analyzeReceipt)
	s.storage.MonthlySummariesTable.GrantReadWriteData(analyzeReceipt)
	analyzeReceipt.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("ssm:GetParameter"),
		Resources: jsii.Strings(
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", s.cfg.Region, s.cfg.AccountID, s.cfg.Parameters.OpenAIAPIKey),
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", s.cfg.Region, s.cfg.AccountID, s.cfg.Parameters.OpenAIModel),
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", s.cfg.Region, s.cfg.AccountID, s.cfg.Parameters.OpenAIReasoningEffort),
		),
	}))

	retryUpload := s.newFunction("retry-upload", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/uploads/{uploadId}/retry", retryUpload)
	s.storage.UploadHistoriesTable.GrantReadWriteData(retryUpload)
	s.storage.AnalyzeQueue.GrantSendMessages(retryUpload)

	listUploads := s.newFunction("list-uploads", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/months/{month}/uploads", listUploads)
	s.storage.UploadHistoriesTable.GrantReadData(listUploads)

	getBilling := s.newFunction("get-billing", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/billings/{billingId}", getBilling)
	s.storage.BillingsTable.GrantReadData(getBilling)
	s.storage.BillingDetailsTable.GrantReadData(getBilling)

	return s
}

// functionProps は newFunction に渡す関数ごとの差分。
type functionProps struct {
	MemorySize float64
	Timeout    awscdk.Duration
	// Environment は共通環境変数に上書き・追加する
	Environment map[string]*string
}

// newFunction は関数専用の ECR イメージから Lambda を作る。
// タグは SSM パラメータ(image:push が更新)を deploy 時に解決するので、関数ごとに別のタイミングで更新できる。
func (s *AppStack) newFunction(name string, props functionProps) awslambda.DockerImageFunction {
	fnCfg, ok := s.cfg.Function(name)
	if !ok {
		panic(fmt.Sprintf("function %q is not in common.FunctionNames", name))
	}
	repo, ok := s.storage.Repositories[name]
	if !ok {
		panic(fmt.Sprintf("storage stack has no repository for function %q", name))
	}
	// ログは Storage スタックが持ち、App を作り直しても残る
	logGroup, ok := s.storage.LogGroups[name]
	if !ok {
		panic(fmt.Sprintf("storage stack has no log group for function %q", name))
	}
	functionName := s.cfg.ResourceName(name)

	env := s.commonEnvironment()
	for k, v := range props.Environment {
		env[k] = v
	}

	// AWS::SSM::Parameter::Value 型の CFN パラメータになる。CDK CLI は SSM 由来のパラメータがあると
	// テンプレートに差分が無くても deploy をスキップしないので、タグの更新だけで反映される
	imageTag := awsssm.StringParameter_ValueForStringParameter(s.Stack, jsii.String(fnCfg.ImageTagParameter), nil)

	return awslambda.NewDockerImageFunction(s.Stack, jsii.String(constructID(name)+"Function"), &awslambda.DockerImageFunctionProps{
		FunctionName: jsii.String(functionName),
		Code: awslambda.DockerImageCode_FromEcr(repo, &awslambda.EcrImageCodeProps{
			TagOrDigest: imageTag,
		}),
		Architecture: awslambda.Architecture_ARM_64(),
		MemorySize:   jsii.Number(props.MemorySize),
		Timeout:      props.Timeout,
		LogGroup:     logGroup,
		Environment:  &env,
	})
}

// commonEnvironment は全 Lambda に渡す環境変数。リソース名と SSM パラメータ名のみで、秘密情報は含めない。
func (s *AppStack) commonEnvironment() map[string]*string {
	st := s.storage
	return map[string]*string{
		common.EnvUsersTable:               st.UsersTable.TableName(),
		common.EnvMonthlySummariesTable:    st.MonthlySummariesTable.TableName(),
		common.EnvUploadHistoriesTable:     st.UploadHistoriesTable.TableName(),
		common.EnvBillingsTable:            st.BillingsTable.TableName(),
		common.EnvBillingDetailsTable:      st.BillingDetailsTable.TableName(),
		common.EnvReceiptBucket:            st.Bucket.BucketName(),
		common.EnvAnalyzeQueueURL:          st.AnalyzeQueue.QueueUrl(),
		common.EnvImageMaxEdge:             jsii.String("2048"),
		common.EnvSSMPasswordPepper:        jsii.String(s.cfg.Parameters.PasswordPepper),
		common.EnvSSMJWTSecret:             jsii.String(s.cfg.Parameters.JWTSecret),
		common.EnvSSMOpenAIAPIKey:          jsii.String(s.cfg.Parameters.OpenAIAPIKey),
		common.EnvSSMOpenAIModel:           jsii.String(s.cfg.Parameters.OpenAIModel),
		common.EnvSSMOpenAIReasoningEffort: jsii.String(s.cfg.Parameters.OpenAIReasoningEffort),
	}
}
