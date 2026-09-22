# infra

AWS CDK (Go) で snap_kakeibo の AWS リソースを定義する。

## スタック

| スタック | 役割 | 中身 |
| --- | --- | --- |
| `{stage}-snap-kakeibo-storage` | 失うと困るもの、人間が成果物を push する先。`Config.RemovalPolicy` が RETAIN の stage では destroy しても残る | ECR(関数ごと)、S3(receipts / frontend)、DynamoDB、Analyze SQS + DLQ、SNS + メール購読、DLQ アラーム |
| `{stage}-snap-kakeibo-app` | destroy して作り直せるもの | Lambda、API Gateway、CloudFront、イベントソースマッピング、ロググループ |
| `{stage}-snap-kakeibo-certificate` | CloudFront 用証明書（`us-east-1`） | ACM 証明書と DNS 検証 |

依存は `app -> storage` と `app -> certificate`。`storage` は `app` を参照しない。`PersonalDNS` が管理する `shrimptail.net` の Hosted Zone は各スタックで ID と名前から参照する。

## ディレクトリ

```text
infra/
├── main.go               // CDK アプリ。storage / certificate / app / pipeline を組み立てる
├── stacks/
│   ├── storage.go        // Storage スタック
│   ├── app.go            // App スタック。newFunction で Lambda を追加する
│   ├── frontend.go       // CloudFront(OAC)。frontend バケットは名前で import する
│   ├── api.go            // HTTP API(v2)、ルート追加、CloudFront /api/* → API の転送
│   ├── certificate.go    // us-east-1 の CloudFront 用 ACM 証明書
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
task infra:bootstrap        # ソウル側の初回のみ
task infra:bootstrap:certificate  # us-east-1 側の初回のみ
```

通常のデプロイは「storage → 人間が成果物を push → certificate → app」の順。CDK は成果物の配置(BucketDeployment / Docker アセット)をしない。

```sh
task infra:deploy:storage                         # 1. SSM パラメータ作成 + storage スタック
task image:push                                   # 2a. 関数ごとにイメージをビルドして専用 ECR へ push し、SSM のタグを更新
task front:push                                   # 2b. front/dist を S3 へ + CloudFront 無効化
task infra:deploy:certificate                     # 3. us-east-1 の CloudFront 証明書（初回・変更時）
task infra:deploy:app                             # 4. app スタック。各 Lambda は SSM のタグを deploy 時に解決する
```

### dev の独自ドメインの初回切替

`shrimptail.net` の Hosted Zone は dotfiles の `PersonalDNS` が管理する。アプリ用の DNS レコードはこのリポジトリで管理する。2026-09-23 に共有された Hosted Zone ID は `Z102143231J5HFGPDMXG8`。デプロイ前に `PersonalDNS` の出力と対象名の空きを再確認する。

| 用途 | 名前 | リージョン |
| --- | --- | --- |
| フロントエンドと `/api/*` の公開 URL | `dev.snap-kakeibo.shrimptail.net` | CloudFront 用 ACM は `us-east-1` |
| CloudFront から API Gateway へのオリジン | `origin-api.dev.snap-kakeibo.shrimptail.net` | API Gateway と ACM は `ap-northeast-2` |

初回は API Gateway の既定 `execute-api` URL と CloudFront の旧オリジンを残して新ドメインを作る。新 API ドメインの疎通を確認してから CloudFront を切り替え、最後に既定 URL を無効化する。`keepExecuteApiEndpoint` と `keepExecuteApiOrigin` はこの切替時のみ使う。既存の S3 CORS は `*` なので、ブラウザ用ドメインの疎通を確認してから Storage スタックで新 origin に絞る。

```sh
task infra:bootstrap:certificate                  # us-east-1 で未実施なら先に実行
task infra:deploy:certificate                     # DNS 検証が終わるまで待つ
task infra:deploy:app -- -c keepExecuteApiEndpoint=true -c keepExecuteApiOrigin=true
# 新 API ドメインの /api/hello と公開 URL の画面・/api/* を確認
task infra:deploy:app -- -c keepExecuteApiEndpoint=true
# CloudFront の新オリジンへの切替完了後、ログイン Cookie・Presigned PUT を確認
task infra:deploy:storage                         # S3 CORS を新しいブラウザ origin に更新
# 新しい URL の Presigned PUT を再確認
task infra:deploy:app                             # 既定 execute-api URL を無効化
```

bootstrap 後は `us-east-1` の CloudFormation 実行ロールに ACM と親 Hosted Zone のレコード変更権限があることを確認する。`VITE_API_BASE_URL` を本番ビルドで設定せず、ブラウザは同一 origin の `/api/*` を呼ぶ。既定 URL は無効化後に呼び出せないことを確認する。オリジン用カスタムドメインへの直接アクセスは、この変更だけでは制限されない。

旧 `*.cloudfront.net` URL は DNS 切替後も開けるが、レシート用 S3 の CORS は新しい URL に限定するため画像アップロードはサポートしない。本番用 `snap-kakeibo.shrimptail.net` は本番環境を定義するときに追加する。

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
| `keepExecuteApiEndpoint` | 初回の CloudFront オリジン切替中だけ既定 API URL を残す | 不要（既定は無効化） |
| `keepExecuteApiOrigin` | 初回切替で CloudFront の旧 `execute-api` オリジンを維持する | 不要（既定は新 API ドメイン） |

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
| `CORSAllowedOrigins` | `https://dev.snap-kakeibo.shrimptail.net` | S3 Presigned PUT を許可するオリジン |

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
