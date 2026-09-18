package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"snap_kakeibo/infra/config"
)

// APIPathPrefix は CloudFront が API Gateway へ転送するパス。React は相対パスで /api/... を呼ぶ。
const APIPathPrefix = "/api"

// newHTTPAPI は Lambda を束ねる HTTP API(v2)を作る。
// 呼び出しは CloudFront の /api/* 経由に統一するので CORS は設定しない。
func newHTTPAPI(scope constructs.Construct, cfg config.Config) awsapigatewayv2.HttpApi {
	api := awsapigatewayv2.NewHttpApi(scope, jsii.String("HttpApi"), &awsapigatewayv2.HttpApiProps{
		ApiName: jsii.String(cfg.ResourceName("api")),
	})
	awscdk.NewCfnOutput(scope, jsii.String("ApiUrl"), &awscdk.CfnOutputProps{Value: api.ApiEndpoint()})
	return api
}

// addRoute は path に Lambda を紐づける。path は /api を含めた完全なパスを渡す。
func addRoute(api awsapigatewayv2.HttpApi, method awsapigatewayv2.HttpMethod, path string, fn awslambda.IFunction) {
	api.AddRoutes(&awsapigatewayv2.AddRoutesOptions{
		Path:        jsii.String(path),
		Methods:     &[]awsapigatewayv2.HttpMethod{method},
		Integration: awsapigatewayv2integrations.NewHttpLambdaIntegration(jsii.String(*fn.Node().Id()+"Integration"), fn, nil),
	})
}

// attachAPIToDistribution は CloudFront の /api/* を HTTP API へ転送する。
// キャッシュはせず、Host 以外のヘッダとクエリをそのまま渡す。
func attachAPIToDistribution(dist awscloudfront.Distribution, api awsapigatewayv2.HttpApi) {
	// ApiEndpoint は https://{id}.execute-api.{region}.amazonaws.com の形なのでホスト部分だけ取り出す
	host := awscdk.Fn_Select(jsii.Number(2), awscdk.Fn_Split(jsii.String("/"), api.ApiEndpoint(), nil))
	origin := awscloudfrontorigins.NewHttpOrigin(host, &awscloudfrontorigins.HttpOriginProps{
		ProtocolPolicy: awscloudfront.OriginProtocolPolicy_HTTPS_ONLY,
	})
	dist.AddBehavior(jsii.String(APIPathPrefix+"/*"), origin, &awscloudfront.AddBehaviorOptions{
		AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachePolicy:          awscloudfront.CachePolicy_CACHING_DISABLED(),
		OriginRequestPolicy:  awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
	})
}
