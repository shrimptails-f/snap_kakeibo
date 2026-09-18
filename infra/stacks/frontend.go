package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/config"
)

// newFrontendDistribution は Storage スタックの frontend バケットを OAC で配信する CloudFront を作る。
// バケットは名前で import する。実バケットを渡すと OAC のバケットポリシーが Storage 側に入り、
// Distribution ARN の逆参照で循環するため。ポリシーは Storage 側で先に付けてある。
func newFrontendDistribution(scope constructs.Construct, cfg config.Config) awscloudfront.Distribution {
	bucket := awss3.Bucket_FromBucketName(scope, jsii.String("FrontendBucket"), jsii.String(cfg.Buckets.Frontend))
	// import したバケットにはポリシーを付けられない旨の警告が出るが、Storage 側で付けてあるので抑止する
	awscdk.Annotations_Of(scope).AcknowledgeWarning(
		jsii.String("@aws-cdk/aws-cloudfront-origins:updateImportedBucketPolicyOac"),
		jsii.String("The frontend bucket policy for CloudFront OAC is defined in the storage stack"),
	)

	dist := awscloudfront.NewDistribution(scope, jsii.String("FrontendDistribution"), &awscloudfront.DistributionProps{
		Comment: jsii.String(cfg.AppStackName + " frontend"),
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin:               awscloudfrontorigins.S3BucketOrigin_WithOriginAccessControl(bucket, nil),
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_GET_HEAD_OPTIONS(),
			CachePolicy:          awscloudfront.CachePolicy_CACHING_OPTIMIZED(),
			Compress:             jsii.Bool(true),
		},
		DefaultRootObject: jsii.String("index.html"),
		// SPA のルーティング。OAC 経由で存在しないキーは 403 になるので両方 index.html に寄せる
		ErrorResponses: &[]*awscloudfront.ErrorResponse{
			spaFallback(403),
			spaFallback(404),
		},
		// 日本を含む価格クラス。PRICE_CLASS_100 は北米・欧州のみで日本からの配信が遠回りになる
		PriceClass:  awscloudfront.PriceClass_PRICE_CLASS_200,
		HttpVersion: awscloudfront.HttpVersion_HTTP2_AND_3,
	})

	awscdk.NewCfnOutput(scope, jsii.String("FrontendUrl"), &awscdk.CfnOutputProps{
		Value: jsii.String("https://" + *dist.DistributionDomainName()),
	})
	awscdk.NewCfnOutput(scope, jsii.String("DistributionId"), &awscdk.CfnOutputProps{
		Value: dist.DistributionId(),
	})
	return dist
}

func spaFallback(httpStatus float64) *awscloudfront.ErrorResponse {
	return &awscloudfront.ErrorResponse{
		HttpStatus:         jsii.Number(httpStatus),
		ResponseHttpStatus: jsii.Number(200),
		ResponsePagePath:   jsii.String("/index.html"),
		Ttl:                awscdk.Duration_Seconds(jsii.Number(0)),
	}
}
