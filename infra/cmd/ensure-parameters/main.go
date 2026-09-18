// ensure-parameters は stage の SSM パラメータが無ければ作る。既存の値は上書きしない。
//
//	go run ./cmd/ensure-parameters -stage dev
//
// pepper と JWT 署名鍵は乱数で生成する。OpenAI の API キーは
// 環境変数 OPENAI_API_KEY があればその値、無ければ UNSET で作る。
// 関数ごとのイメージタグは UNSET で作り、image:push が上書きする。
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"snap_kakeibo/infra/common"
	"snap_kakeibo/infra/config"
)

func main() {
	stage := flag.String("stage", string(common.StageDev), "deployment stage")
	flag.Parse()

	cfg, err := config.Load(*stage)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		log.Fatal(err)
	}

	if err := ensureAll(ctx, ssm.NewFromConfig(awsCfg), parametersFor(cfg)); err != nil {
		log.Fatal(err)
	}
}

type parameter struct {
	Name  string
	Type  types.ParameterType
	Value string
}

// parametersFor は stage に必要なパラメータと初期値を返す。
func parametersFor(cfg config.Config) []parameter {
	params := []parameter{
		{Name: cfg.Parameters.PasswordPepper, Type: types.ParameterTypeSecureString, Value: randomSecret()},
		{Name: cfg.Parameters.JWTSecret, Type: types.ParameterTypeSecureString, Value: randomSecret()},
		{Name: cfg.Parameters.OpenAIAPIKey, Type: types.ParameterTypeSecureString, Value: envOr("OPENAI_API_KEY", common.UnsetParameterValue)},
		{Name: cfg.CI.Parameters.GitHubConnectionARN, Type: types.ParameterTypeString, Value: common.UnsetParameterValue},
		{Name: cfg.CI.Parameters.BackendLastSuccessfulCommit, Type: types.ParameterTypeString, Value: common.UnsetParameterValue},
		{Name: cfg.CI.Parameters.FrontendLastSuccessfulCommit, Type: types.ParameterTypeString, Value: common.UnsetParameterValue},
		{Name: cfg.CI.Parameters.InfraLastSuccessfulCommit, Type: types.ParameterTypeString, Value: common.UnsetParameterValue},
	}
	for _, f := range cfg.Functions {
		params = append(params, parameter{Name: f.ImageTagParameter, Type: types.ParameterTypeString, Value: common.UnsetParameterValue})
	}
	return params
}

// parameterStore は ssm.Client のうち使う操作。テストで差し替える
type parameterStore interface {
	GetParameter(ctx context.Context, in *ssm.GetParameterInput, opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	PutParameter(ctx context.Context, in *ssm.PutParameterInput, opts ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

// ensureAll は無いパラメータだけを作る。
func ensureAll(ctx context.Context, store parameterStore, params []parameter) error {
	for _, p := range params {
		created, err := ensure(ctx, store, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p.Name, err)
		}
		if created {
			log.Printf("created %s (%s)", p.Name, p.Type)
		} else {
			log.Printf("exists  %s", p.Name)
		}
	}
	return nil
}

func ensure(ctx context.Context, store parameterStore, p parameter) (created bool, err error) {
	_, err = store.GetParameter(ctx, &ssm.GetParameterInput{Name: &p.Name})
	if err == nil {
		return false, nil
	}
	var notFound *types.ParameterNotFound
	if !errors.As(err, &notFound) {
		return false, err
	}

	_, err = store.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      &p.Name,
		Type:      p.Type,
		Value:     &p.Value,
		Tier:      types.ParameterTierStandard,
		Overwrite: boolPtr(false),
	})
	if err != nil {
		// 存在確認と作成の間に作られた場合は既存を尊重する
		var exists *types.ParameterAlreadyExists
		if errors.As(err, &exists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// randomSecret は 32 バイトの乱数を base64 で返す。Argon2id の pepper と HS256 の署名鍵に使う。
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return base64.RawStdEncoding.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolPtr(b bool) *bool { return &b }
