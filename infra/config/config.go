// Package config は stage ごとの設定を持つ。型は config.go、値は stage 名のファイル(dev.go など)で定義する。
package config

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"

	"snap_kakeibo/infra/common"
)

// Config は1つの stage のインフラ設定。
type Config struct {
	Stage     common.Stage
	AccountID string
	Region    string
	// AlertEmail は DLQ アラートの通知先。空なら購読を作らない
	AlertEmail string
	// CORSAllowedOrigins は S3 Presigned PUT を許可するオリジン
	CORSAllowedOrigins []string
	// RemovalPolicy は stateful リソース(DynamoDB / S3 / ECR)をスタック削除時に残すか。
	// RETAIN なら削除保護も付ける。DESTROY なら中身ごと消せるようにする
	RemovalPolicy awscdk.RemovalPolicy

	StorageStackName string
	AppStackName     string
	Functions        []Function
	Buckets          Buckets
	Tables           Tables
	Queues           Queues
	Topics           Topics
	Parameters       Parameters
	Timeouts         Timeouts
	// MaxReceiveCount は一時エラーの再試行回数。超過したメッセージは DLQ へ移る
	MaxReceiveCount float64
}

// Function は Lambda 1つ分のデプロイ設定。関数ごとに ECR リポジトリを持ち、デプロイタイミングを分けられる。
type Function struct {
	Name       string
	Repository ECRConfig
	// ImageTagParameter はデプロイ中のイメージタグを持つ SSM パラメータ名
	ImageTagParameter string
	// LogGroup は Lambda のロググループ名。Storage スタックが作る
	LogGroup string
}

// ECRConfig は ECR リポジトリの設定。
type ECRConfig struct {
	Name string
	// MaxImageCount を超えた古いイメージはライフサイクルルールで消える
	MaxImageCount float64
	// ImageTagMutability が IMMUTABLE なら同じタグの再 push はエラーになる
	ImageTagMutability awsecr.TagMutability
}

// functions は common.FunctionNames から関数ごとの設定を組み立てる。ECR の設定は全関数で共通。
func functions(stage common.Stage, repository ECRConfig) []Function {
	fs := make([]Function, 0, len(common.FunctionNames))
	for _, name := range common.FunctionNames {
		repo := repository
		repo.Name = common.ResourceName(name).For(stage)
		fs = append(fs, Function{
			Name:              name,
			Repository:        repo,
			ImageTagParameter: common.ImageTagParameterName(name).For(stage),
			LogGroup:          common.LogGroupName(name).For(stage),
		})
	}
	return fs
}

// Buckets は S3 バケット名。S3 の名前はグローバルに一意なので衝突したら stage 側で変える。
type Buckets struct {
	// Receipts はレシート画像と OpenAI 生レスポンス JSON
	Receipts string
	// Frontend は React のビルド成果物。CloudFront(OAC) から配信する
	Frontend string
}

// Tables は DynamoDB テーブル名。
type Tables struct {
	Users            string
	MonthlySummaries string
	UploadHistories  string
	Billings         string
	BillingDetails   string
}

// Queues は SQS キュー名。
type Queues struct {
	Analyze    string
	AnalyzeDLQ string
}

// Topics は SNS トピック名。
type Topics struct {
	Alert string
}

// Parameters は SSM パラメータ名。値は CDK で作らず手動投入する。
type Parameters struct {
	PasswordPepper        string
	JWTSecret             string
	OpenAIAPIKey          string
	OpenAIModel           string
	OpenAIReasoningEffort string
}

// Timeouts は Lambda のタイムアウト。SQS の可視性タイムアウトはこの6倍で決まるため両スタックで共有する。
type Timeouts struct {
	Analyze awscdk.Duration
	API     awscdk.Duration
}

// Load は stage 名に対応する設定を返す。
func Load(stage string) (Config, error) {
	var cfg Config
	switch common.Stage(stage) {
	case common.StageDev:
		cfg = Dev()
	default:
		return Config{}, fmt.Errorf("unknown stage %q", stage)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Stage == "" || c.AccountID == "" || c.Region == "" {
		return fmt.Errorf("stage, account ID, and region are required")
	}
	if c.StorageStackName == "" || c.AppStackName == "" {
		return fmt.Errorf("stack names are required")
	}
	if c.RemovalPolicy == "" {
		return fmt.Errorf("removal policy is required")
	}
	if len(c.Functions) == 0 {
		return fmt.Errorf("at least one function is required")
	}
	for _, f := range c.Functions {
		if f.Name == "" || f.ImageTagParameter == "" || f.LogGroup == "" {
			return fmt.Errorf("function name and image tag parameter are required")
		}
		if f.Repository.Name == "" || f.Repository.MaxImageCount <= 0 || f.Repository.ImageTagMutability == "" {
			return fmt.Errorf("%s: repository name, max image count, and tag mutability are required", f.Name)
		}
	}
	return nil
}

// Function は名前で関数設定を引く。
func (c Config) Function(name string) (Function, bool) {
	for _, f := range c.Functions {
		if f.Name == name {
			return f, true
		}
	}
	return Function{}, false
}

// Retain は stateful リソースをスタック削除時に残す設定かどうか。
func (c Config) Retain() bool {
	return c.RemovalPolicy != awscdk.RemovalPolicy_DESTROY
}

// ResourceName は stage 付きのリソース名を返す。Lambda 関数名などスタック内で動的に決まる名前に使う。
func (c Config) ResourceName(name string) string {
	return common.ResourceName(name).For(c.Stage)
}
