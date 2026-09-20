// Package ssmtest はローカル(Floci)の SSM Parameter Store にテストごとの一時パラメータを作るヘルパーを提供する。
//
// 名前は毎回ランダムに決まり、テスト終了時に t.Cleanup で削除される。
//
//	env := dynamodbtest.Connect(t)                                   // STAGE が local / ci でなければ Skip
//	name := ssmtest.PutSecureString(t, env.Config, "jwt-secret", "x") // "/test-jwt-secret-<random>" を作り、終了時に消す
//
// Lambda のバイナリに testing パッケージを混ぜないため、ssm 本体とは別パッケージにしている。
package ssmtest

import (
	"context"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/awstest"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// timeout は作成・削除 1 回の上限。
const timeout = 30 * time.Second

// PutSecureString はランダムな名前("/test-" + prefix + 乱数)の SecureString パラメータを作り、その名前を返す。
// テスト終了時に削除する。
func PutSecureString(t testing.TB, cfg aws.Config, prefix, value string) string {
	t.Helper()
	name := "/" + awstest.ResourceName("test-"+prefix)
	client := awssdk.NewFromConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := client.PutParameter(ctx, &awssdk.PutParameterInput{Name: aws.String(name), Value: aws.String(value), Type: types.ParameterTypeSecureString}); err != nil {
		t.Fatalf("ssmtest: put parameter %s against %s: %v", name, aws.ToString(cfg.BaseEndpoint), err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if _, err := client.DeleteParameter(ctx, &awssdk.DeleteParameterInput{Name: aws.String(name)}); err != nil {
			t.Errorf("ssmtest: delete parameter %s: %v", name, err)
		}
	})
	return name
}
