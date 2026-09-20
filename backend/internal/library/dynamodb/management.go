package dynamodb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ManagerAPI は Manager が使う DynamoDB のテーブル管理操作。*awssdk.Client が満たす。
// Lambda の実行時には不要なので、読み書き用の API とは分けている。
type ManagerAPI interface {
	CreateTable(ctx context.Context, params *awssdk.CreateTableInput, optFns ...func(*awssdk.Options)) (*awssdk.CreateTableOutput, error)
	DeleteTable(ctx context.Context, params *awssdk.DeleteTableInput, optFns ...func(*awssdk.Options)) (*awssdk.DeleteTableOutput, error)
	DescribeTable(ctx context.Context, params *awssdk.DescribeTableInput, optFns ...func(*awssdk.Options)) (*awssdk.DescribeTableOutput, error)
}

var _ ManagerAPI = (*awssdk.Client)(nil)

// Manager はテーブルの作成・削除を行う。ローカル(Floci)での開発とテストでの利用を想定しており、
// 本番のテーブルは CDK(infra/stacks/storage.go)が作るので Lambda からは使わない。
type Manager struct {
	api ManagerAPI
	// waitTimeout は CreateTable / DeleteTable の完了待ちの上限
	waitTimeout time.Duration
}

// DefaultWaitTimeout はテーブルが ACTIVE / 削除完了になるまで待つ既定の上限。
const DefaultWaitTimeout = time.Minute

// NewManager は cfg から Manager を生成する。
func NewManager(cfg aws.Config) *Manager { return NewManagerWithAPI(awssdk.NewFromConfig(cfg)) }

// NewManagerWithAPI はテスト用の API 差し替えを受け取って Manager を生成する。
func NewManagerWithAPI(api ManagerAPI) *Manager {
	return &Manager{api: api, waitTimeout: DefaultWaitTimeout}
}

// WithWaitTimeout は完了待ちの上限を変えた Manager を返す。
func (m *Manager) WithWaitTimeout(d time.Duration) *Manager {
	return &Manager{api: m.api, waitTimeout: d}
}

// Cleanup は CreateTable が返す後始末。呼ぶとテーブルを削除し、消えるまで待つ。
// 既に消えていれば何もしないので、複数回呼んでも安全。
type Cleanup func(ctx context.Context) error

// CreateTable は name でスキーマ通りのテーブルを作り、ACTIVE になるまで待つ。
// 戻り値の Cleanup を defer で呼ぶと作ったテーブルを削除できる。
// 既に同名のテーブルがあればエラー。テストではランダムな名前(RandomTableName)を使う。
//
//	cleanup, err := manager.CreateTable(ctx, name, UsersSchema)
//	if err != nil { ... }
//	defer cleanup(ctx)
func (m *Manager) CreateTable(ctx context.Context, name string, schema Schema) (Cleanup, error) {
	if err := ValidateTableName(name); err != nil {
		return nil, err
	}
	in, err := schema.CreateTableInput(name)
	if err != nil {
		return nil, err
	}
	if _, err := m.api.CreateTable(ctx, in); err != nil {
		return nil, fmt.Errorf("dynamodb: create table %s: %w", name, err)
	}
	waiter := awssdk.NewTableExistsWaiter(m.api, func(o *awssdk.TableExistsWaiterOptions) {
		o.MinDelay, o.MaxDelay = waiterMinDelay, waiterMaxDelay
	})
	if err := waiter.Wait(ctx, &awssdk.DescribeTableInput{TableName: aws.String(name)}, m.waitTimeout); err != nil {
		return nil, fmt.Errorf("dynamodb: wait for table %s to become active: %w", name, err)
	}
	return func(ctx context.Context) error { return m.DeleteTable(ctx, name) }, nil
}

// DeleteTable は name のテーブルを削除し、消えるまで待つ。存在しなければ何もしない。
func (m *Manager) DeleteTable(ctx context.Context, name string) error {
	if err := ValidateTableName(name); err != nil {
		return err
	}
	if _, err := m.api.DeleteTable(ctx, &awssdk.DeleteTableInput{TableName: aws.String(name)}); err != nil {
		if IsTableNotFound(err) {
			return nil
		}
		return fmt.Errorf("dynamodb: delete table %s: %w", name, err)
	}
	waiter := awssdk.NewTableNotExistsWaiter(m.api, func(o *awssdk.TableNotExistsWaiterOptions) {
		o.MinDelay, o.MaxDelay = waiterMinDelay, waiterMaxDelay
	})
	if err := waiter.Wait(ctx, &awssdk.DescribeTableInput{TableName: aws.String(name)}, m.waitTimeout); err != nil {
		return fmt.Errorf("dynamodb: wait for table %s to be deleted: %w", name, err)
	}
	return nil
}

// TableExists は name のテーブルがあれば true を返す。
func (m *Manager) TableExists(ctx context.Context, name string) (bool, error) {
	if _, err := m.api.DescribeTable(ctx, &awssdk.DescribeTableInput{TableName: aws.String(name)}); err != nil {
		if IsTableNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("dynamodb: describe table %s: %w", name, err)
	}
	return true, nil
}

// IsTableNotFound はテーブルが存在しないエラーなら true。
func IsTableNotFound(err error) bool {
	var notFound *types.ResourceNotFoundException
	return errors.As(err, &notFound)
}

// SDK の waiter の待ち間隔。既定(20 秒)は Floci には長すぎるので短くする。
const (
	waiterMinDelay = 200 * time.Millisecond
	waiterMaxDelay = 2 * time.Second
)

// テーブル名の制約(DynamoDB の仕様)。
const (
	tableNameMinLen = 3
	tableNameMaxLen = 255
)

var tableNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// ValidateTableName は DynamoDB のテーブル名の制約(3〜255 文字、英数字と _ . -)を確認する。
func ValidateTableName(name string) error {
	if len(name) < tableNameMinLen || len(name) > tableNameMaxLen || !tableNamePattern.MatchString(name) {
		return fmt.Errorf("dynamodb: invalid table name %q (want %d-%d chars of [a-zA-Z0-9_.-])", name, tableNameMinLen, tableNameMaxLen)
	}
	return nil
}

// randomSuffixBytes はランダム接尾辞のバイト長(16 進で 2 倍の文字数になる)。
const randomSuffixBytes = 8

// RandomTableName はテストごとに衝突しないテーブル名を返す。
//
//	RandomTableName("scenario", UsersSchema) // "scenario-users-3f9a2c1e8b7d6a54"
//
// prefix はテストの種類などを表す任意の文字列。空なら "test" を使う。
// 並列に走るテストや途中で落ちて残ったテーブルと衝突しないよう、接尾辞は crypto/rand で作る。
func RandomTableName(prefix string, schema Schema) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "test"
	}
	buf := make([]byte, randomSuffixBytes)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand の失敗は OS 側の異常で、テーブル名の衝突より深刻なので止める
		panic(fmt.Sprintf("dynamodb: random table name: %v", err))
	}
	return fmt.Sprintf("%s-%s-%s", prefix, schema.Name, hex.EncodeToString(buf))
}
