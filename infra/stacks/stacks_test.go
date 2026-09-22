package stacks

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

// このパッケージのテストは t.Parallel() を呼ばない。
// jsii-runtime-go は Node プロセスへのリクエストを排他せず、Node 側のカーネルも単一スレッド前提なので、
// 並行に呼ぶと data race / segfault になる(awscdk.NewApp や config.Dev() 内の Duration_Minutes も jsii 経由)。
func synth(t *testing.T, cfg config.Config) (assertions.Template, assertions.Template) {
	t.Helper()
	app := awscdk.NewApp(nil)
	storage := NewStorageStack(app, cfg.StorageStackName, &StorageStackProps{Config: cfg})
	appStack := NewAppStack(app, cfg.AppStackName, &AppStackProps{Config: cfg, Storage: storage})
	return assertions.Template_FromStack(storage.Stack, nil), assertions.Template_FromStack(appStack.Stack, nil)
}

func synthPipeline(t *testing.T, cfg config.Config) assertions.Template {
	t.Helper()
	app := awscdk.NewApp(nil)
	pipeline := NewPipelineStack(app, cfg.PipelineStackName, &PipelineStackProps{Config: cfg})
	return assertions.Template_FromStack(pipeline.Stack, nil)
}

func TestStorageStackResources(t *testing.T) {
	cfg := config.Dev()
	cfg.AlertEmail = "alert@example.com"
	storage, app := synth(t, cfg)

	// 関数ごとに ECR を持つ
	storage.ResourceCountIs(jsii.String("AWS::ECR::Repository"), jsii.Number(len(cfg.Functions)))
	storage.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(2))
	storage.ResourceCountIs(jsii.String("AWS::DynamoDB::Table"), jsii.Number(6))
	storage.ResourceCountIs(jsii.String("AWS::SQS::Queue"), jsii.Number(2))
	storage.ResourceCountIs(jsii.String("AWS::SNS::Topic"), jsii.Number(1))
	storage.ResourceCountIs(jsii.String("AWS::SNS::Subscription"), jsii.Number(1))
	storage.ResourceCountIs(jsii.String("AWS::CloudWatch::Alarm"), jsii.Number(1))
	storage.ResourceCountIs(jsii.String("AWS::Logs::LogGroup"), jsii.Number(len(cfg.Functions)))
	storage.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), map[string]any{
		"LogGroupName":    "dev-snap-kakeibo-hello_lambda",
		"RetentionInDays": 30,
	})

	storage.HasResourceProperties(jsii.String("AWS::ECR::Repository"), map[string]any{
		"RepositoryName":     "dev-snap-kakeibo-hello",
		"ImageTagMutability": "IMMUTABLE",
		"LifecyclePolicy": map[string]any{
			"LifecyclePolicyText": assertions.Match_StringLikeRegexp(jsii.String(`"countNumber":10`)),
		},
	})
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"TableName": cfg.Tables.AnalysisRequests,
		"GlobalSecondaryIndexes": []any{map[string]any{
			"IndexName": common.AnalysisRequestMonthIndex,
		}},
	})
	// 支出と支出明細は expenses / expense-details。明細は月ごとに金額順で引く GSI を持つ
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"TableName": cfg.Tables.Expenses,
	})
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"TableName": cfg.Tables.ExpenseDetails,
		"GlobalSecondaryIndexes": []any{map[string]any{
			"IndexName": common.DetailMonthAmountIndex,
		}},
	})
	// refresh token は専用テーブル。期限切れは TTL、ユーザー単位の一括失効は GSI で行う
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"TableName": cfg.Tables.RefreshTokens,
		"KeySchema": []any{map[string]any{"AttributeName": "PK", "KeyType": "HASH"}},
		"TimeToLiveSpecification": map[string]any{
			"AttributeName": "expires_at",
			"Enabled":       true,
		},
		"GlobalSecondaryIndexes": []any{map[string]any{
			"IndexName": common.RefreshTokenUserIndex,
		}},
	})
	// Analyze Lambda 3分 -> 可視性タイムアウト 18分
	storage.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]any{
		"QueueName":         "dev-snap-kakeibo-analyze",
		"VisibilityTimeout": 1080,
	})
	storage.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]any{
		"QueueName": "dev-snap-kakeibo-analyze-dlq",
	})
	storage.HasResourceProperties(jsii.String("AWS::S3::Bucket"), map[string]any{"LifecycleConfiguration": map[string]any{"Rules": assertions.Match_ArrayWith(&[]any{assertions.Match_ObjectLike(&map[string]any{"Prefix": "analysis-results/", "ExpirationInDays": 90})})}})
	storage.HasResourceProperties(jsii.String("AWS::SNS::Subscription"), map[string]any{
		"Protocol": "email",
		"Endpoint": "alert@example.com",
	})

	// frontend バケットは CloudFront OAC から読めるようにしておく(Distribution は App 側)
	storage.HasResourceProperties(jsii.String("AWS::S3::BucketPolicy"), map[string]any{
		"PolicyDocument": map[string]any{
			"Statement": assertions.Match_ArrayWith(&[]any{assertions.Match_ObjectLike(&map[string]any{
				"Sid":       "AllowCloudFrontOAC",
				"Principal": map[string]any{"Service": "cloudfront.amazonaws.com"},
				"Condition": map[string]any{"ArnLike": map[string]any{
					"AWS:SourceArn": "arn:aws:cloudfront::" + cfg.AccountID + ":distribution/*",
				}},
			})}),
		},
	})

	// App 側はバケットを import するだけで作らない
	app.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(0))
	app.ResourceCountIs(jsii.String("AWS::CloudFront::Distribution"), jsii.Number(1))
	app.ResourceCountIs(jsii.String("AWS::CloudFront::OriginAccessControl"), jsii.Number(1))
	app.HasResourceProperties(jsii.String("AWS::CloudFront::Distribution"), map[string]any{
		"DistributionConfig": assertions.Match_ObjectLike(&map[string]any{
			"CacheBehaviors": assertions.Match_ArrayWith(&[]any{assertions.Match_ObjectLike(&map[string]any{
				"PathPattern": "/api/*",
			})}),
		}),
	})

	// hello Lambda は専用 ECR のイメージから作り、タグは SSM パラメータを deploy 時に解決する
	app.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Api"), jsii.Number(1))
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{
		"RouteKey": "GET /api/hello",
	})
	app.ResourceCountIs(jsii.String("AWS::Lambda::Alias"), jsii.Number(len(cfg.Functions)))
	app.ResourceCountIs(jsii.String("AWS::CodeDeploy::Application"), jsii.Number(1))
	app.ResourceCountIs(jsii.String("AWS::CodeDeploy::DeploymentConfig"), jsii.Number(1))
	app.ResourceCountIs(jsii.String("AWS::CodeDeploy::DeploymentGroup"), jsii.Number(len(cfg.Functions)))
	app.HasResourceProperties(jsii.String("AWS::CodeDeploy::Application"), map[string]any{
		"ApplicationName": "dev-snap-kakeibo-lambda",
		"ComputePlatform": "Lambda",
	})
	app.HasResourceProperties(jsii.String("AWS::CodeDeploy::DeploymentConfig"), map[string]any{
		"DeploymentConfigName": "dev-snap-kakeibo-lambda-all-at-once",
		"TrafficRoutingConfig": map[string]any{"Type": "AllAtOnce"},
	})
	app.HasResourceProperties(jsii.String("AWS::Lambda::Alias"), map[string]any{
		"Name": "live",
		"FunctionName": map[string]any{
			"Ref": assertions.Match_StringLikeRegexp(jsii.String("^HelloFunction")),
		},
	})
	app.HasResource(jsii.String("AWS::Lambda::Alias"), map[string]any{
		"UpdatePolicy": map[string]any{
			"CodeDeployLambdaAliasUpdate": assertions.Match_AnyValue(),
		},
	})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Integration"), map[string]any{
		"IntegrationUri": map[string]any{
			"Ref": assertions.Match_StringLikeRegexp(jsii.String("^HelloLiveAlias")),
		},
	})
	app.HasResourceProperties(jsii.String("AWS::CodeDeploy::DeploymentGroup"), map[string]any{
		"DeploymentGroupName": "dev-snap-kakeibo-hello-deployment-group",
		"DeploymentConfigName": map[string]any{
			"Ref": assertions.Match_StringLikeRegexp(jsii.String("^LambdaDeploymentConfig")),
		},
		"DeploymentStyle": map[string]any{
			"DeploymentOption": "WITH_TRAFFIC_CONTROL",
			"DeploymentType":   "BLUE_GREEN",
		},
	})
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName":  "dev-snap-kakeibo-hello",
		"PackageType":   "Image",
		"Architectures": []any{"arm64"},
		// ImageUri は ImportValue(ECR URI) + ":" + Ref(SSM 型 CFN パラメータ) の Fn::Join になる
		"Code": map[string]any{
			"ImageUri": map[string]any{"Fn::Join": []any{"", assertions.Match_ArrayWith(&[]any{
				map[string]any{"Ref": assertions.Match_StringLikeRegexp(jsii.String("^SsmParameterValue"))},
			})}},
		},
	})
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{"FunctionName": "dev-snap-kakeibo-analyze-receipt", "MemorySize": 1024, "Timeout": 180})
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName": "dev-snap-kakeibo-analyze-receipt",
		"Environment": map[string]any{"Variables": assertions.Match_ObjectLike(&map[string]any{
			"OPENAI_MODEL": "gpt-5-mini", "OPENAI_REASONING_EFFORT": "low",
		})},
	})
	// SQS イベントソースは live Alias に付け、CodeDeploy の切り替えで解析側も更新できるようにする
	app.HasResourceProperties(jsii.String("AWS::Lambda::EventSourceMapping"), map[string]any{
		"BatchSize":     1,
		"ScalingConfig": map[string]any{"MaximumConcurrency": 5},
		// Alias 経由だと FunctionName は "<関数名>:live" の Fn::Join になる
		"FunctionName": map[string]any{"Fn::Join": []any{"", assertions.Match_ArrayWith(&[]any{":live"})}},
	})
	app.HasResourceProperties(jsii.String("AWS::CodeDeploy::DeploymentGroup"), map[string]any{
		"DeploymentGroupName": "dev-snap-kakeibo-analyze-receipt-deployment-group",
	})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{"RouteKey": "POST /api/analysis-requests/{analysisRequestId}/retry"})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{"RouteKey": "GET /api/months/{month}/analysis-requests"})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{"RouteKey": "GET /api/expenses/{expenseId}"})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{"RouteKey": "PATCH /api/expenses/{expenseId}"})
	app.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), map[string]any{"RouteKey": "POST /api/monthly-summaries/{month}/rebuild"})
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName": "dev-snap-kakeibo-update-expense",
		"Environment": map[string]any{"Variables": assertions.Match_ObjectLike(&map[string]any{
			"EXPENSES_TABLE": assertions.Match_AnyValue(), "EXPENSE_DETAILS_TABLE": assertions.Match_AnyValue(), "MONTHLY_SUMMARIES_TABLE": assertions.Match_AnyValue(),
		})},
	})
	// 環境変数のテーブル名も新名称で渡す
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName": "dev-snap-kakeibo-get-expense",
		"Environment": map[string]any{"Variables": assertions.Match_ObjectLike(&map[string]any{
			"EXPENSES_TABLE":          assertions.Match_AnyValue(),
			"EXPENSE_DETAILS_TABLE":   assertions.Match_AnyValue(),
			"ANALYSIS_REQUESTS_TABLE": assertions.Match_AnyValue(),
			"RECEIPT_BUCKET":          assertions.Match_AnyValue(),
		})},
	})
	app.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]any{
		"Roles": assertions.Match_ArrayWith(&[]any{map[string]any{
			"Ref": assertions.Match_StringLikeRegexp(jsii.String("^GetExpenseFunctionServiceRole")),
		}}),
		"PolicyDocument": map[string]any{"Statement": assertions.Match_ArrayWith(&[]any{assertions.Match_ObjectLike(&map[string]any{
			"Action": "s3:GetObject",
			"Effect": "Allow",
		})})},
	})
	app.HasParameter(jsii.String("*"), map[string]any{
		"Type":    "AWS::SSM::Parameter::Value<String>",
		"Default": "/dev/snap-kakeibo/functions/hello/image-tag",
	})
	// ロググループは Storage が持ち、Lambda は Export 経由で参照する
	app.ResourceCountIs(jsii.String("AWS::Logs::LogGroup"), jsii.Number(0))
	app.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]any{
		"FunctionName": "dev-snap-kakeibo-hello",
		"LoggingConfig": map[string]any{
			"LogGroup": map[string]any{"Fn::ImportValue": assertions.Match_AnyValue()},
		},
	})
	app.HasResourceProperties(jsii.String("AWS::CloudFront::Distribution"), map[string]any{
		"DistributionConfig": assertions.Match_ObjectLike(&map[string]any{
			"DefaultRootObject": "index.html",
			"PriceClass":        "PriceClass_200",
			"CustomErrorResponses": assertions.Match_ArrayWith(&[]any{assertions.Match_ObjectLike(&map[string]any{
				"ErrorCode": 403, "ResponseCode": 200, "ResponsePagePath": "/index.html", "ErrorCachingMinTTL": 0,
			})}),
		}),
	})
}

func TestDevDestroysStatefulResources(t *testing.T) {
	storage, _ := synth(t, config.Dev())

	for _, typ := range []string{"AWS::DynamoDB::Table", "AWS::S3::Bucket", "AWS::ECR::Repository", "AWS::Logs::LogGroup"} {
		storage.HasResource(jsii.String(typ), map[string]any{
			"DeletionPolicy": "Delete",
		})
	}
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"DeletionProtectionEnabled": false,
	})
	storage.HasResourceProperties(jsii.String("AWS::ECR::Repository"), map[string]any{
		"EmptyOnDelete": true,
	})
	// S3 の AutoDeleteObjects はバケットごとのカスタムリソースで実現される(receipts / frontend)
	storage.ResourceCountIs(jsii.String("Custom::S3AutoDeleteObjects"), jsii.Number(2))
}

func TestRetainPolicyProtectsStatefulResources(t *testing.T) {
	cfg := config.Dev()
	cfg.RemovalPolicy = awscdk.RemovalPolicy_RETAIN
	storage, _ := synth(t, cfg)

	for _, typ := range []string{"AWS::DynamoDB::Table", "AWS::S3::Bucket", "AWS::ECR::Repository", "AWS::Logs::LogGroup"} {
		storage.HasResource(jsii.String(typ), map[string]any{
			"DeletionPolicy": "Retain",
		})
	}
	storage.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"DeletionProtectionEnabled": true,
	})
	storage.ResourceCountIs(jsii.String("Custom::S3AutoDeleteObjects"), jsii.Number(0))
}

func TestAlertEmailIsOptional(t *testing.T) {
	cfg := config.Dev()
	cfg.AlertEmail = ""
	storage, _ := synth(t, cfg)
	storage.ResourceCountIs(jsii.String("AWS::SNS::Subscription"), jsii.Number(0))
}

func TestPipelineStackResources(t *testing.T) {
	cfg := config.Dev()
	pipeline := synthPipeline(t, cfg)

	pipeline.ResourceCountIs(jsii.String("AWS::CodePipeline::Pipeline"), jsii.Number(2))
	pipeline.ResourceCountIs(jsii.String("AWS::CodeBuild::Project"), jsii.Number(2))
	pipeline.HasResourceProperties(jsii.String("AWS::CodePipeline::Pipeline"), map[string]any{
		"Name": "dev-snap-kakeibo-backend",
		"Stages": assertions.Match_ArrayWith(&[]any{
			assertions.Match_ObjectLike(&map[string]any{
				"Name": "Source",
				"Actions": assertions.Match_ArrayWith(&[]any{
					assertions.Match_ObjectLike(&map[string]any{
						"Configuration": assertions.Match_ObjectLike(&map[string]any{
							"BranchName":           "deploy/dev",
							"FullRepositoryId":     "shrimptails-f/snap_kakeibo",
							"OutputArtifactFormat": "CODEBUILD_CLONE_REF",
						}),
					}),
				}),
			}),
			assertions.Match_ObjectLike(&map[string]any{"Name": "BuildAndDeploy"}),
		}),
	})
	pipeline.HasResourceProperties(jsii.String("AWS::CodeBuild::Project"), map[string]any{
		"Name": "dev-snap-kakeibo-backend-build-and-deploy",
		"Environment": assertions.Match_ObjectLike(&map[string]any{
			"PrivilegedMode": true,
		}),
	})
	pipeline.HasResourceProperties(jsii.String("AWS::CodeBuild::Project"), map[string]any{
		"Name": "dev-snap-kakeibo-frontend-build-and-deploy",
		"Environment": assertions.Match_ObjectLike(&map[string]any{
			"PrivilegedMode": false,
		}),
	})
	pipeline.HasParameter(jsii.String("*"), map[string]any{
		"Type":    "AWS::SSM::Parameter::Value<String>",
		"Default": "/dev/snap-kakeibo/cicd/github-connection-arn",
	})
}

func TestUnknownStageIsRejected(t *testing.T) {
	if _, err := config.Load("nope"); err == nil {
		t.Fatal("expected an error for an unknown stage")
	}
}

func TestNamesIncludeStage(t *testing.T) {
	if got, want := common.UsersTableName.Dev(), "dev-snap-kakeibo-users"; got != want {
		t.Errorf("UsersTableName.Dev() = %q, want %q", got, want)
	}
	if got, want := common.OpenAIAPIKeyParameterName.Dev(), "/dev/snap-kakeibo/openai/api-key"; got != want {
		t.Errorf("OpenAIAPIKeyParameterName.Dev() = %q, want %q", got, want)
	}
	if got, want := config.Dev().StorageStackName, "dev-snap-kakeibo-storage"; got != want {
		t.Errorf("StorageStackName = %q, want %q", got, want)
	}
}
