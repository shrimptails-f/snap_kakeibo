package stacks

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

func synth(t *testing.T, cfg config.Config) (assertions.Template, assertions.Template) {
	t.Helper()
	app := awscdk.NewApp(nil)
	storage := NewStorageStack(app, cfg.StorageStackName, &StorageStackProps{Config: cfg})
	appStack := NewAppStack(app, cfg.AppStackName, &AppStackProps{Config: cfg, Storage: storage})
	return assertions.Template_FromStack(storage.Stack, nil), assertions.Template_FromStack(appStack.Stack, nil)
}

func TestStorageStackResources(t *testing.T) {
	cfg := config.Dev()
	cfg.AlertEmail = "alert@example.com"
	storage, app := synth(t, cfg)

	// 関数ごとに ECR を持つ
	storage.ResourceCountIs(jsii.String("AWS::ECR::Repository"), jsii.Number(len(cfg.Functions)))
	storage.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(2))
	storage.ResourceCountIs(jsii.String("AWS::DynamoDB::Table"), jsii.Number(5))
	storage.ResourceCountIs(jsii.String("AWS::SQS::Queue"), jsii.Number(4))
	storage.ResourceCountIs(jsii.String("AWS::SNS::Topic"), jsii.Number(2))
	storage.ResourceCountIs(jsii.String("AWS::SNS::Subscription"), jsii.Number(2))
	storage.ResourceCountIs(jsii.String("AWS::CloudWatch::Alarm"), jsii.Number(2))
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
		"TableName": cfg.Tables.UploadHistories,
		"GlobalSecondaryIndexes": []any{map[string]any{
			"IndexName": common.UploadMonthIndex,
		}},
	})
	// StartTextract Lambda 60秒 -> 可視性タイムアウト 360秒
	storage.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]any{
		"QueueName":         "dev-snap-kakeibo-start-textract",
		"VisibilityTimeout": 360,
	})
	storage.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]any{
		"QueueName": "dev-snap-kakeibo-start-textract-dlq",
	})
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
	// SNS -> SQS の購読のみ
	storage.ResourceCountIs(jsii.String("AWS::SNS::Subscription"), jsii.Number(1))
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
