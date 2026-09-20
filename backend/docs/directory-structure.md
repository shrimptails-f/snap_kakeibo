# Backend ディレクトリ構成

## 全体構成

```text
backend/
├── cmd/                         # Lambda ごとのエントリポイント
│   └── <lambda-name>/
│       ├── main.go              # 設定読み込み、DI、handler 登録
│       └── main_test.go         # entrypoint 固有処理のテスト（必要な場合）
├── internal/                    # backend モジュール内部の実装
│   ├── app/                     # HTTP response、既存共通 DTO・設定
│   ├── common/
│   │   └── domain/              # 複数 feature で共有するドメイン型
│   ├── <feature>/               # 業務 feature 単位の package
│   │   ├── application/         # usecase、入出力、依存 interface、エラー
│   │   ├── domain/              # entity、value object、不変条件
│   │   ├── infrastructure/      # DynamoDB など外部接続の実装
│   │   └── library/             # feature 内に閉じる技術要素
│   ├── library/                 # feature をまたいで使う技術基盤
│   │   └── <library-name>/
│   └── di/                      # 共通および Lambda ごとの依存注入
├── docs/                        # backend の設計・実装規約
├── tools/                       # 開発・運用補助コマンド
│   └── <tool-name>/
│       └── main.go
├── Dockerfile                   # Lambda コンテナイメージのビルド
├── go.mod                       # Go module 定義
└── go.sum                       # 依存 module のチェックサム
```

テストは対象コードと同じ package の `*_test.go` に置く。共有サービスを使う統合テストも、対象 package の近くに配置し、ファイル名やコメントで統合テストであることを明確にする。

## `cmd/<lambda-name>`

各ディレクトリはデプロイ単位となる Lambda と1対1で対応する。

`main.go` の責務は次に限定する。

- 起動時設定の読み込みと検証
- AWS client、logger、DI container の構築
- application usecase の解決
- Lambda event と application input/output の変換
- application error と HTTP response の対応付け
- `lambdawrap` を使った handler 登録

業務判断、永続化処理、token 検証などの主要ロジックは置かない。Lambda 名を追加・変更する場合は、`infra/common.FunctionNames` との一致も確認する。

## `internal/<feature>`

業務機能を feature 単位でまとめる。すべての feature が4ディレクトリを必ず持つ必要はなく、実際に責務があるものだけ作る。

```text
internal/<feature>/
├── application/
│   ├── interfaces.go            # usecase が必要とする依存契約
│   ├── errors.go                # 呼び出し側が判定する application error
│   ├── <usecase>.go
│   └── <usecase>_test.go
├── domain/
│   ├── <model>.go
│   └── <model>_test.go
├── infrastructure/
│   ├── <resource>_repository.go
│   └── <resource>_repository_test.go
└── library/
    └── <technical-concern>/     # cookie、password、settings、token など
```

### `application`

- usecase とその input/output を置く。
- repository、clock、token verifier など、usecase が必要とする最小限の interface を定義する。
- AWS SDK、Lambda event、HTTP response など外部境界の型へ依存しない。

### `domain`

- feature 固有の entity、value object、不変条件を置く。
- 永続化や HTTP の都合だけで必要な field/tag は持ち込まない。
- 複数 feature で共有すべき型だけ `internal/common/domain` へ移す。

### `infrastructure`

- application が定義した interface の実装を置く。
- DynamoDB のキー構築、attribute 変換、条件式など外部サービス固有の詳細を閉じ込める。
- SDK client は `internal/library` の wrapper がある場合、それを利用する。

### feature 内の `library`

- その feature に閉じる技術要素を置く。
- 複数 feature から利用されることが確定してから、共通の `internal/library` への移動を検討する。

## 共通 package

### `internal/app`

Lambda の HTTP 境界で共有する response helper、既存 DTO、移行中の共通設定を置く。新しい業務ロジックや feature 固有モデルの置き場所にはしない。

### `internal/common/domain`

複数 feature が同じ意味と不変条件で利用するドメイン型を置く。単なるコード量削減を目的に feature 固有型を移さない。

### `internal/library`

```text
internal/library/<library-name>/
├── client.go                    # client や wrapper の実装
├── interface.go                 # package 自身が公開すべき契約（必要な場合）
└── *_test.go
```

AWS SDK、logger、trace、retry、時刻、環境変数などの技術基盤を置く。業務ルールは置かない。上位レイヤーから差し替える契約は、原則としてその契約を利用する application 側で定義する。

### `internal/di`

```text
internal/di/
├── dig.go                       # 全 Lambda で使う共通依存
├── <feature>.go                 # feature 共通の provider
├── <lambda_name>.go             # Lambda 固有の provider と resolver
└── *_test.go
```

共通 container を作成した後、Lambda ごとに必要な infrastructure/library 実装と application usecase を登録する。業務処理は DI provider 内へ書かない。

## 依存方向

基本の依存方向は次のとおり。

```text
cmd/<lambda>
  └── di
      ├── application
      ├── infrastructure ──┐
      └── library          │
                           ▼
application ──> domain / common/domain
infrastructure ──> application interface + library
```

- application/domain から `cmd`、DI、infrastructure、AWS SDK へ依存しない。
- HTTP や Lambda event の型は `cmd` の境界で application の input/output に変換する。
- package 間の循環依存を作らない。
- 詳細な実装規約は [`coding_rules.md`](./coding_rules.md) を参照する。
