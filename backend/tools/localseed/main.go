// localseed は Floci にローカル開発用のリソース(DynamoDB テーブル・S3 バケット・SQS キュー・SSM の JWT 署名鍵)と
// ログイン用の利用者を作る。何度実行しても同じ状態に収束する(既にあるものは作り直さず、利用者は上書きする)。
//
//	go run ./tools/localseed                                  # dev@example.com / password で利用者を作る
//	go run ./tools/localseed -email me@example.com -password secret
//
// リソース名は tools/localenv で決め、tools/localapi が同じ名前を Lambda に渡す。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/library/awsconfig"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/ulid"
	"snap_kakeibo/backend/tools/localenv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"golang.org/x/crypto/bcrypt"
)

const timeout = 2 * time.Minute

func main() {
	email := flag.String("email", "dev@example.com", "ログインに使うメールアドレス")
	password := flag.String("password", "password", "ログインに使うパスワード(Floci 専用のダミー)")
	flag.Parse()

	if err := run(*email, *password); err != nil {
		fmt.Fprintln(os.Stderr, "localseed:", err)
		os.Exit(1)
	}
}

func run(email, password string) error {
	if err := localenv.EnsureLocalProcessEnv(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cfg, err := awsconfig.Load(ctx, oswrapper.New())
	if err != nil {
		return err
	}
	fmt.Printf("endpoint: %s\n", aws.ToString(cfg.BaseEndpoint))

	if err := ensureTables(ctx, cfg); err != nil {
		return err
	}
	if err := ensureBucket(ctx, cfg); err != nil {
		return err
	}
	queueURL, err := ensureQueue(ctx, cfg)
	if err != nil {
		return err
	}
	if err := ensureJWTSecret(ctx, cfg); err != nil {
		return err
	}
	if err := putUser(ctx, cfg, email, password); err != nil {
		return err
	}
	fmt.Printf("queue:    %s\nuser:     %s / %s\n", queueURL, email, password)
	return nil
}

func ensureTables(ctx context.Context, cfg aws.Config) error {
	manager := libdynamodb.NewManager(cfg)
	for _, schema := range libdynamodb.AllSchemas() {
		name := localenv.TableName(schema)
		exists, err := manager.TableExists(ctx, name)
		if err != nil {
			return err
		}
		if exists {
			fmt.Printf("table:    %s (exists)\n", name)
			continue
		}
		if _, err := manager.CreateTable(ctx, name, schema); err != nil {
			return err
		}
		fmt.Printf("table:    %s (created)\n", name)
	}
	return nil
}

func ensureBucket(ctx context.Context, cfg aws.Config) error {
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })
	if _, err := client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(localenv.ReceiptBucket)}); err == nil {
		fmt.Printf("bucket:   %s (exists)\n", localenv.ReceiptBucket)
		return nil
	}
	if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{
		Bucket:                    aws.String(localenv.ReceiptBucket),
		CreateBucketConfiguration: &s3types.CreateBucketConfiguration{LocationConstraint: s3types.BucketLocationConstraint(cfg.Region)},
	}); err != nil {
		return fmt.Errorf("create bucket %s: %w", localenv.ReceiptBucket, err)
	}
	fmt.Printf("bucket:   %s (created)\n", localenv.ReceiptBucket)
	return nil
}

// ensureQueue は analyze キューを作り、その URL を返す。CreateQueue は同じ名前なら既存の URL を返すので冪等。
func ensureQueue(ctx context.Context, cfg aws.Config) (string, error) {
	out, err := awssqs.NewFromConfig(cfg).CreateQueue(ctx, &awssqs.CreateQueueInput{QueueName: aws.String(localenv.AnalyzeQueueName)})
	if err != nil {
		return "", fmt.Errorf("create queue %s: %w", localenv.AnalyzeQueueName, err)
	}
	return aws.ToString(out.QueueUrl), nil
}

// ensureJWTSecret は署名鍵が無ければランダムに作る。作り直すと発行済みの access token が無効になるので上書きしない。
func ensureJWTSecret(ctx context.Context, cfg aws.Config) error {
	client := awsssm.NewFromConfig(cfg)
	_, err := client.GetParameter(ctx, &awsssm.GetParameterInput{Name: aws.String(localenv.JWTSecretParameter)})
	if err == nil {
		fmt.Printf("ssm:      %s (exists)\n", localenv.JWTSecretParameter)
		return nil
	}
	var notFound *ssmtypes.ParameterNotFound
	if !errors.As(err, &notFound) {
		return fmt.Errorf("get parameter %s: %w", localenv.JWTSecretParameter, err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	if _, err := client.PutParameter(ctx, &awsssm.PutParameterInput{
		Name:  aws.String(localenv.JWTSecretParameter),
		Value: aws.String(hex.EncodeToString(secret)),
		Type:  ssmtypes.ParameterTypeSecureString,
	}); err != nil {
		return fmt.Errorf("put parameter %s: %w", localenv.JWTSecretParameter, err)
	}
	fmt.Printf("ssm:      %s (created)\n", localenv.JWTSecretParameter)
	return nil
}

// putUser は test/auth の seedUser と同じ形で利用者を書く。既にあれば user_id を引き継いでパスワードだけ更新する。
func putUser(ctx context.Context, cfg aws.Config, email, password string) error {
	users := libdynamodb.New(cfg, nil).Table(localenv.TableName(libdynamodb.UsersSchema))
	key := map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPKByEmail(email)}}

	userID, err := existingUserID(ctx, users, key)
	if err != nil {
		return err
	}
	if userID == "" {
		if userID, err = ulid.New(nil).NewID(); err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = users.PutItem(ctx, &awsdynamodb.PutItemInput{Item: map[string]ddbtypes.AttributeValue{
		"PK":            key["PK"],
		"user_id":       &ddbtypes.AttributeValueMemberS{Value: userID},
		"email":         &ddbtypes.AttributeValueMemberS{Value: email},
		"password_hash": &ddbtypes.AttributeValueMemberS{Value: string(hash)},
	}})
	if err != nil {
		return fmt.Errorf("put user: %w", err)
	}
	return nil
}

func existingUserID(ctx context.Context, users *libdynamodb.Table, key map[string]ddbtypes.AttributeValue) (string, error) {
	out, err := users.GetItem(ctx, &awsdynamodb.GetItemInput{Key: key})
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}
	id, ok := out.Item["user_id"].(*ddbtypes.AttributeValueMemberS)
	if !ok {
		return "", nil
	}
	return id.Value, nil
}
