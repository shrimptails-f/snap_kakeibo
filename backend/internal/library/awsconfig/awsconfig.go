// Package awsconfig は実行環境(STAGE)に応じた aws.Config を組み立てる。
//
// SDK クライアントの向き先(エンドポイント・資格情報)は生成時に決まるので、各サービスの
// ラッパーではなくここで 1 回だけ決める。全 Lambda は main で Load を呼び、その Config から
// DynamoDB / S3 / SQS などのクライアントを作る。
//
//   - STAGE=local / ci: AWS_ENDPOINT_URL(Floci)へ向け、固定のダミー資格情報を使う。
//     ~/.aws や IAM ロールには触れないので、誤って本物の AWS に繋がらない
//   - それ以外(dev / stg / prd)と STAGE 未設定: SDK の既定の解決(環境変数・IAM ロールなど)
//
// STAGE 未設定を許すのは、Lambda に STAGE が渡っていなくても動くようにするため。
package awsconfig

import (
	"context"
	"fmt"

	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/stage"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Floci が参照する環境変数。
const (
	EnvEndpointURL = "AWS_ENDPOINT_URL"
	EnvRegion      = "AWS_REGION"
)

// localCredential は Floci 向けのダミー資格情報。Floci は値を検証しない。
const localCredential = "test"

// Load は STAGE に応じた aws.Config を返す。
func Load(ctx context.Context, osw oswrapper.Interface) (aws.Config, error) {
	return load(ctx, osw, func(ctx context.Context) (aws.Config, error) {
		return config.LoadDefaultConfig(ctx)
	})
}

func load(ctx context.Context, osw oswrapper.Interface, loadDefault func(context.Context) (aws.Config, error)) (aws.Config, error) {
	raw, err := osw.GetEnv(stage.EnvKey)
	if err != nil {
		// 未設定は AWS 上(Lambda)とみなす
		return loadDefault(ctx)
	}
	st, err := stage.Parse(raw)
	if err != nil {
		return aws.Config{}, err
	}
	if !st.IsLocal() {
		return loadDefault(ctx)
	}
	return loadLocal(osw)
}

// loadLocal は Floci 向けの Config を作る。共有設定ファイルやプロファイルは読まない。
func loadLocal(osw oswrapper.Interface) (aws.Config, error) {
	endpoint, err := osw.GetEnv(EnvEndpointURL)
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: local stage needs %s (Floci の URL): %w", EnvEndpointURL, err)
	}
	region, err := osw.GetEnv(EnvRegion)
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: local stage needs %s: %w", EnvRegion, err)
	}
	return aws.Config{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(localCredential, localCredential, ""),
	}, nil
}
