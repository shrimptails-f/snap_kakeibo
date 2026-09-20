// Package di は Lambda ごとの依存性を合成する。
package di

import (
	"fmt"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/retry"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	libsqs "snap_kakeibo/backend/internal/library/sqs"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/dig"
)

// NewContainer はすべての Lambda で使う共通依存性を登録したコンテナを返す。
// Lambda ごとの DI 定義はこのコンテナへ業務固有の provider を追加する。
func NewContainer(awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) (*dig.Container, error) {
	container := dig.New()
	if err := ProvideCommonDependencies(container, awsCfg, osw, log); err != nil {
		return nil, err
	}
	return container, nil
}

// ProvideCommonDependencies は feature 固有の識別子・秘密値・動作設定なしで生成できる library 実装を登録する。
//
// ここで登録する AWS client は未束縛である。バケット名、キュー URL、テーブル名、SSM parameter 名は
// feature 側の DI で与え、Client.Bucket / Queue / Table / Parameter に束縛する。
// API key のような feature 固有の秘密値が必須の client はここに登録しない。
func ProvideCommonDependencies(container *dig.Container, awsCfg aws.Config, osw oswrapper.Interface, log logger.Interface) error {
	for _, provider := range []any{
		func() aws.Config { return awsCfg },
		func() oswrapper.Interface { return osw },
		func() logger.Interface { return log },
		libdynamodb.New,
		libssm.New,
		libs3.New,
		libsqs.New,
		func() timewrapper.Interface { return timewrapper.NewClock() },
		retry.New,
	} {
		if err := container.Provide(provider); err != nil {
			return fmt.Errorf("provide common dependency: %w", err)
		}
	}
	return nil
}

// ResolveLogger はコンテナから Lambda 用 logger を取り出す。
func ResolveLogger(container *dig.Container) (logger.Interface, error) {
	var log logger.Interface
	if err := container.Invoke(func(resolved logger.Interface) { log = resolved }); err != nil {
		return nil, fmt.Errorf("resolve logger: %w", err)
	}
	return log, nil
}

// Provide は Lambda 固有の provider を登録する共通ヘルパー。
func Provide(container *dig.Container, scope string, providers ...any) error {
	for _, provider := range providers {
		if err := container.Provide(provider); err != nil {
			return fmt.Errorf("provide %s dependency: %w", scope, err)
		}
	}
	return nil
}
