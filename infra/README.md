# infra

AWS CDK (Go) で snap_kakeibo の AWS リソースを定義する。

## スタック

| スタック | 役割 | 中身 |
| --- | --- | --- |
| `{stage}-snap-kakeibo-storage` | 失うと困るもの、人間が成果物を push する先。`Config.RemovalPolicy` が RETAIN の stage では destroy しても残る | ECR(関数ごと)、S3(receipts / frontend)、DynamoDB、Analyze SQS + DLQ、SNS + メール購読、DLQ アラーム |
| `{stage}-snap-kakeibo-app` | destroy して作り直せるもの | Lambda、API Gateway、CloudFront、イベントソースマッピング、ロググループ |

依存は `app -> storage` の一方向のみ。`storage` は `app` を参照しない。

## ディレクトリ

```text
infra/
├── main.go               // CDK アプリ。2スタックを組み立てる
├── stacks/
│   ├── storage.go        // Storage スタック
│   ├── app.go            // App スタック。newFunction で Lambda を追加する
│   ├── frontend.go       // CloudFront(OAC)。frontend バケットは名前で import する
│   ├── api.go            // HTTP API(v2)、ルート追加、CloudFront /api/* → API の転送
│   └── stacks_test.go    // テンプレートのアサーション
├── internal/config/      // リソース名、SSM パラメータ名、環境変数名、タイムアウト
├── cdk.json              // CDK 設定と feature flags
└── package.json          // CDK CLI のバージョン固定
```

## コマンド

ルートの Taskfile から実行する。実 AWS を触るタスクは `AWS_ENDPOINT_URL` を外してホストのプロファイルを使う。

```sh
task infra:setup            # Go 依存の導入
task infra:test             # スタックのアサーション
task infra:synth            # テンプレート生成のみ
task infra:bootstrap        # 初回のみ
```

デプロイは「storage → 人間が成果物を push → app」の順。CDK は成果物の配置(BucketDeployment / Docker アセット)をしない。

```sh
task infra:deploy:storage                         # 1. SSM パラメータ作成 + storage スタック
task image:push                                   # 2a. 関数ごとにイメージをビルドして専用 ECR へ push し、SSM のタグを更新
task front:push                                   # 2b. front/dist を S3 へ + CloudFront 無効化
task infra:deploy:app                             # 3. app スタック。各 Lambda は SSM のタグを deploy 時に解決する
```

### Lambda のデプロイ単位

関数ごとに ECR リポジトリ(`{stage}-snap-kakeibo-{name}`)と、デプロイ中のタグを持つ SSM パラメータ
(`/{stage}/snap-kakeibo/functions/{name}/image-tag`)を持つ。関数ごとにデプロイのタイミングを分けられる。

```sh
FUNCTIONS="upload" task image:push   # upload だけビルド・push・SSM 更新(既定は backend/cmd の全関数)
task infra:deploy:app                # SSM のタグが変わった関数だけ更新される
```

- `IMAGE_TAG` は既定で `git rev-parse --short HEAD`。`IMAGE_TAG=<sha>` で上書きできる
- 同じタグが ECR に既にあればビルドと push は飛ばし、SSM の更新だけ行う
- `infra:deploy:app` は先に全関数のタグが `UNSET` でないことを確認する(`scripts/check-image-tags.sh`)
- 関数を足すときは `backend/cmd/{name}` と `common.FunctionNames` の両方に追加する(テストで一致を確認している)。storage を deploy し直すと ECR と SSM ができる
- `image:push` は Docker が必要。開発コンテナには Docker ソケットを渡していないので、ホストか CI で実行する

stage は `STAGE=prd task infra:deploy:storage` のように環境変数で切り替える(既定 dev)。

## Context

| キー | 用途 | 必須 |
| --- | --- | --- |
| `stage` | 使う設定(`config/{stage}.go`)。Taskfile が `STAGE` から渡す。`cdk.json` の既定値は `dev` | 必須(既定値あり) |

stage ごとの値(メール通知先、CORS オリジンなど)は context ではなく `config/{stage}.go` に書く。

## リソース名

stage 抜きの名前を `common` に値オブジェクトの定数として置き、stage を付けた実名は `.Dev()` / `.For(stage)` で取る。

```go
common.UsersTableName.Dev()            // "dev-snap-kakeibo-users"
common.OpenAIAPIKeyParameterName.Dev() // "/dev/snap-kakeibo/openai/api-key"
common.ResourceName("upload").For(stage) // "{stage}-snap-kakeibo-upload"(関数の ECR / Lambda 名)
```

stage を増やすときは `common.Stage` に定数を足し、`config/{stage}.go` を `dev.go` と同じ形で作って `Load` に登録する。
DLQ アラートの通知先 `AlertEmail` は空だと購読を作らず警告だけ出す。設定して初回 deploy した後、確認メールを承認する。

## stage ごとの設定値

`config/{stage}.go` で決める主な値。

| 項目 | dev | 意味 |
| --- | --- | --- |
| `RemovalPolicy` | `DESTROY` | DynamoDB / S3 / ECR をスタック削除時に残すか。RETAIN なら DynamoDB の削除保護も付く。DESTROY なら S3 は自動で空にし、ECR はイメージごと消す |
| `Functions[].Repository.MaxImageCount` | 10 | 各 ECR に残す世代数。超えた古いイメージはライフサイクルルールで消える |
| `Functions[].Repository.ImageTagMutability` | `IMMUTABLE` | 同じタグの再 push をエラーにする |
| `AlertEmail` | 未設定 | DLQ アラートの通知先 |
| `CORSAllowedOrigins` | `*` | S3 Presigned PUT を許可するオリジン |

## Lambda イメージの規約

- 関数ごとに 1 イメージ。`backend/Dockerfile` に `FUNCTION_NAME` を渡して `cmd/{name}` を `/var/runtime/bootstrap` に置く
- ベースイメージは `public.ecr.aws/lambda/provided:al2023`、Lambda は arm64
- タグは git SHA。ECR が `IMMUTABLE` の stage では同じタグを push し直せない(`latest` は使わない)

## SSM Parameter Store

CloudFormation は SecureString を作れないため、CDK ではなく `cmd/ensure-parameters` が deploy 前に作る。
`task infra:deploy` が自動で呼ぶ。単体で実行するなら `task infra:parameters`(別 stage は `STAGE=xxx task infra:parameters`)。

| パラメータ | 型 | 初期値 |
| --- | --- | --- |
| `/{stage}/snap-kakeibo/auth/password-pepper` | SecureString | 32 バイトの乱数 |
| `/{stage}/snap-kakeibo/auth/jwt-secret` | SecureString | 32 バイトの乱数 |
| `/{stage}/snap-kakeibo/openai/api-key` | SecureString | 環境変数 `OPENAI_API_KEY`、無ければ `UNSET` |

既存のパラメータは値・型とも上書きしない。値を変えるときは手動で更新する。

OpenAI のモデルと reasoning effort は非機密値のため、`config/dev.go` から Analyze Lambda の `OPENAI_MODEL` / `OPENAI_REASONING_EFFORT` 環境変数へ渡す。

```sh
env -u AWS_ENDPOINT_URL aws ssm put-parameter --overwrite \
  --name /dev/snap-kakeibo/openai/api-key --type SecureString --value 'sk-...'
```

CDK のスタックには含まれないので `cdk destroy` しても残る。
