package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscodedeploy"
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

	codeDeployApplication awscodedeploy.LambdaApplication
	deploymentConfig      awscodedeploy.ILambdaDeploymentConfig
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
	s.codeDeployApplication = awscodedeploy.NewLambdaApplication(stack, jsii.String("LambdaCodeDeployApplication"), &awscodedeploy.LambdaApplicationProps{
		ApplicationName: jsii.String(props.Config.ResourceName("lambda")),
	})
	s.deploymentConfig = s.newLambdaDeploymentConfig()

	// 疎通確認。各ステップの Lambda はこの形で追加していく
	hello := s.newFunction("hello", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/hello", hello.Handler)

	authLogin := s.newFunction("auth-login", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/auth/login", authLogin.Handler)
	// users は利用者の取得とログイン試行カウンタの更新、refresh-tokens は発行分の書き込み
	s.storage.UsersTable.GrantReadWriteData(authLogin.Handler)
	s.storage.RefreshTokensTable.GrantWriteData(authLogin.Handler)
	s.grantJWTSecretRead(authLogin.Function)

	authRefresh := s.newFunction("auth-refresh", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/auth/refresh", authRefresh.Handler)
	// users は利用者の再取得のみ。refresh token の照合・失効・再発行は refresh-tokens で行う
	s.storage.UsersTable.GrantReadData(authRefresh.Handler)
	s.storage.RefreshTokensTable.GrantReadWriteData(authRefresh.Handler)
	s.grantJWTSecretRead(authRefresh.Function)

	authLogout := s.newFunction("auth-logout", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/auth/logout", authLogout.Handler)
	s.storage.RefreshTokensTable.GrantWriteData(authLogout.Handler)

	authCheck := s.newFunction("auth-check", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/auth/check", authCheck.Handler)
	s.grantJWTSecretRead(authCheck.Function)

	upload := s.newFunction("upload", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/uploads", upload.Handler)
	s.storage.UploadHistoriesTable.GrantWriteData(upload.Handler)
	s.storage.Bucket.GrantPut(upload.Handler, jsii.String("receipts/*"))
	s.grantJWTSecretRead(upload.Function)

	// SQS 起動だが live Alias にイベントソースを付け、API 関数と同じく CodeDeploy で切り替える
	analyzeReceipt := s.newFunction("analyze-receipt", functionProps{MemorySize: 1024, Timeout: props.Config.Timeouts.Analyze, CodeDeploy: true})
	analyzeReceipt.Handler.AddEventSource(awslambdaeventsources.NewSqsEventSource(s.storage.AnalyzeQueue, &awslambdaeventsources.SqsEventSourceProps{
		BatchSize:      jsii.Number(1),
		MaxConcurrency: jsii.Number(5),
	}))
	s.storage.Bucket.GrantRead(analyzeReceipt.Handler, jsii.String("receipts/*"))
	s.storage.Bucket.GrantPut(analyzeReceipt.Handler, jsii.String("analysis-results/*"))
	s.storage.UploadHistoriesTable.GrantReadWriteData(analyzeReceipt.Handler)
	s.storage.BillingsTable.GrantReadWriteData(analyzeReceipt.Handler)
	s.storage.BillingDetailsTable.GrantReadWriteData(analyzeReceipt.Handler)
	s.storage.MonthlySummariesTable.GrantReadWriteData(analyzeReceipt.Handler)
	analyzeReceipt.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("ssm:GetParameter"),
		Resources: jsii.Strings(
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", s.cfg.Region, s.cfg.AccountID, s.cfg.Parameters.OpenAIAPIKey),
		),
	}))

	retryUpload := s.newFunction("retry-upload", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_POST, APIPathPrefix+"/uploads/{uploadId}/retry", retryUpload.Handler)
	s.storage.UploadHistoriesTable.GrantReadWriteData(retryUpload.Handler)
	s.storage.AnalyzeQueue.GrantSendMessages(retryUpload.Handler)
	s.grantJWTSecretRead(retryUpload.Function)

	listUploads := s.newFunction("list-uploads", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/months/{month}/uploads", listUploads.Handler)
	s.storage.UploadHistoriesTable.GrantReadData(listUploads.Handler)
	s.grantJWTSecretRead(listUploads.Function)

	getBilling := s.newFunction("get-billing", functionProps{MemorySize: 256, Timeout: props.Config.Timeouts.API, CodeDeploy: true})
	addRoute(s.API, awsapigatewayv2.HttpMethod_GET, APIPathPrefix+"/billings/{billingId}", getBilling.Handler)
	s.storage.BillingsTable.GrantReadData(getBilling.Handler)
	s.storage.BillingDetailsTable.GrantReadData(getBilling.Handler)
	s.grantJWTSecretRead(getBilling.Function)

	return s
}

func (s *AppStack) grantJWTSecretRead(fn awslambda.IFunction) {
	fn.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: jsii.Strings("ssm:GetParameter"),
		Resources: jsii.Strings(
			fmt.Sprintf("arn:aws:ssm:%s:%s:parameter%s", s.cfg.Region, s.cfg.AccountID, s.cfg.Parameters.JWTSecret),
		),
	}))
}

// functionProps は newFunction に渡す関数ごとの差分。
type functionProps struct {
	MemorySize float64
	Timeout    awscdk.Duration
	// CodeDeploy を true にした関数は呼び出し元(API Gateway / SQS)を live Alias に向け、CodeDeploy が alias を更新する。
	CodeDeploy bool
	// Environment は共通環境変数に上書き・追加する
	Environment map[string]*string
}

type deployedFunction struct {
	Function awslambda.DockerImageFunction
	Handler  awslambda.IFunction
	Alias    awslambda.Alias
}

// newFunction は関数専用の ECR イメージから Lambda を作る。
// タグは SSM パラメータ(image:push が更新)を deploy 時に解決するので、関数ごとに別のタイミングで更新できる。
func (s *AppStack) newFunction(name string, props functionProps) deployedFunction {
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

	fn := awslambda.NewDockerImageFunction(s.Stack, jsii.String(constructID(name)+"Function"), &awslambda.DockerImageFunctionProps{
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

	deployed := deployedFunction{Function: fn, Handler: fn}
	if !props.CodeDeploy {
		return deployed
	}

	id := constructID(name)
	alias := awslambda.NewAlias(s.Stack, jsii.String(id+"LiveAlias"), &awslambda.AliasProps{
		AliasName: jsii.String("live"),
		Version:   fn.CurrentVersion(),
	})
	awscodedeploy.NewLambdaDeploymentGroup(s.Stack, jsii.String(id+"DeploymentGroup"), &awscodedeploy.LambdaDeploymentGroupProps{
		Application:         s.codeDeployApplication,
		Alias:               alias,
		DeploymentConfig:    s.deploymentConfig,
		DeploymentGroupName: jsii.String(s.cfg.ResourceName(name + "-deployment-group")),
	})

	deployed.Handler = alias
	deployed.Alias = alias
	return deployed
}

func (s *AppStack) newLambdaDeploymentConfig() awscodedeploy.ILambdaDeploymentConfig {
	props := &awscodedeploy.LambdaDeploymentConfigProps{
		DeploymentConfigName: jsii.String(s.cfg.ResourceName("lambda-" + string(s.cfg.Deployment.Strategy))),
	}
	switch s.cfg.Deployment.Strategy {
	case config.DeploymentStrategyAllAtOnce:
		props.TrafficRouting = awscodedeploy.TrafficRouting_AllAtOnce()
	case config.DeploymentStrategyCanary:
		props.TrafficRouting = awscodedeploy.TrafficRouting_TimeBasedCanary(&awscodedeploy.TimeBasedCanaryTrafficRoutingProps{
			Percentage: jsii.Number(s.cfg.Deployment.CanaryPercent),
			Interval:   awscdk.Duration_Minutes(jsii.Number(s.cfg.Deployment.IntervalMinutes)),
		})
	case config.DeploymentStrategyLinear:
		props.TrafficRouting = awscodedeploy.TrafficRouting_TimeBasedLinear(&awscodedeploy.TimeBasedLinearTrafficRoutingProps{
			Percentage: jsii.Number(s.cfg.Deployment.CanaryPercent),
			Interval:   awscdk.Duration_Minutes(jsii.Number(s.cfg.Deployment.IntervalMinutes)),
		})
	default:
		panic(fmt.Sprintf("unknown deployment strategy %q", s.cfg.Deployment.Strategy))
	}
	return awscodedeploy.NewLambdaDeploymentConfig(s.Stack, jsii.String("LambdaDeploymentConfig"), props)
}

// commonEnvironment は全 Lambda に渡す環境変数。リソース名と SSM パラメータ名のみで、秘密情報は含めない。
func (s *AppStack) commonEnvironment() map[string]*string {
	st := s.storage
	return map[string]*string{
		common.EnvStage:                 jsii.String(string(s.cfg.Stage)),
		common.EnvUsersTable:            st.UsersTable.TableName(),
		common.EnvRefreshTokensTable:    st.RefreshTokensTable.TableName(),
		common.EnvMonthlySummariesTable: st.MonthlySummariesTable.TableName(),
		common.EnvUploadHistoriesTable:  st.UploadHistoriesTable.TableName(),
		common.EnvBillingsTable:         st.BillingsTable.TableName(),
		common.EnvBillingDetailsTable:   st.BillingDetailsTable.TableName(),
		common.EnvReceiptBucket:         st.Bucket.BucketName(),
		common.EnvAnalyzeQueueURL:       st.AnalyzeQueue.QueueUrl(),
		common.EnvImageMaxEdge:          jsii.String("2048"),
		common.EnvSSMPasswordPepper:     jsii.String(s.cfg.Parameters.PasswordPepper),
		common.EnvSSMJWTSecret:          jsii.String(s.cfg.Parameters.JWTSecret),
		common.EnvSSMOpenAIAPIKey:       jsii.String(s.cfg.Parameters.OpenAIAPIKey),
		common.EnvOpenAIModel:           jsii.String(s.cfg.OpenAI.Model),
		common.EnvOpenAIReasoningEffort: jsii.String(s.cfg.OpenAI.ReasoningEffort),
	}
}
