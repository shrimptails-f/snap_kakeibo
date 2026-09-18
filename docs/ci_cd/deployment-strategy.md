# デプロイ戦略

## 採用方針

API Gateway 経由の Lambda は、Lambda 関数本体ではなく `live` Alias に向ける。

コードデプロイは CodeDeploy for Lambda で行い、本番相当の環境では `LambdaCanary10Percent5Minutes` を使う。

```text
API Gateway HTTP API
  |
  v
Lambda alias: live
  |-- old version 90%
  `-- new version 10%

5分後

Lambda alias: live
  |
  v
new version 100%
```

## 環境ごとの設定

CodeDeploy の Canary / Linear の待ち時間は分単位で扱う。秒単位の 5秒 / 10秒 canary は CodeDeploy 標準では表現しない。

| stage | 戦略 | 目的 |
| --- | --- | --- |
| `dev` | `AllAtOnce` | CodeDeploy 経路は通しつつ、待ち時間をなくす |
| `stg` | `Canary 10% / 1分` | 短時間で canary の流れを確認する |
| `prod` | `Canary 10% / 5分` | 本番相当の段階デプロイ |

`dev` でも直更新には戻さない。待ち時間だけを短くし、Version / Alias / CodeDeploy の構成は全環境で揃える。

## 責務分担

CDK はデプロイの仕組みを管理する。

```text
CDK が管理:
  API Gateway
  Lambda function
  Lambda alias: live
  CodeDeploy Application
  CodeDeploy Deployment Group
  CodeDeploy Deployment Config
  IAM
  ECR
  S3
  DynamoDB
  SQS
  CloudWatch Alarm
```

通常のコードデプロイでは `cdk deploy app` を実行しない。CodeBuild が新しい Lambda Version を発行し、CodeDeploy deployment を開始する。

```text
CodeBuild が実行:
  ECR push
  lambda update-function-code --publish
  CodeDeploy AppSpec 生成
  deploy create-deployment
  deployment 成功待ち
  last-successful-commit 更新
```

## 対象 Lambda

まず API Gateway 同期呼び出しの Lambda を CodeDeploy 対象にする。

| 関数 | 方針 |
| --- | --- |
| `hello` | 対象。疎通確認用 |
| `upload` | 対象。DynamoDB / S3 / SQS への副作用に注意 |
| `retry-upload` | 対象。SQS メッセージ互換性に注意 |
| `list-uploads` | 対象。読み取り系 |
| `get-billing` | 対象。読み取り系 |
| `analyze-receipt` | 初期対象外。SQS 起動のため別途検討する |

`analyze-receipt` は OpenAI 呼び出しと DynamoDB 更新を伴う非同期処理なので、まずは DLQ / error / duration 監視を整えてから Alias / CodeDeploy 対象にするか決める。

## 実装方針

stage ごとの deployment config は `config/{stage}.go` で切り替える。

```go
type DeploymentConfig struct {
	Strategy        DeploymentStrategy
	CanaryPercent  float64
	IntervalMinutes float64
}

const (
	DeploymentStrategyAllAtOnce = "all-at-once"
	DeploymentStrategyCanary    = "canary"
	DeploymentStrategyLinear    = "linear"
)
```

現在の CDK 実装では、API Gateway 同期呼び出し対象の Lambda に `live` Alias と CodeDeploy Deployment Group を作り、HTTP API の Integration は Alias を参照する。`dev` は `AllAtOnce` のカスタム Deployment Config を使う。

CD の流れ:

```text
1. 変更対象の関数を判定する
2. 対象関数をテストする
3. 対象関数のイメージを ECR へ push する
4. aws lambda update-function-code --image-uri ... --publish
5. 発行された新 Version を取得する
6. live Alias の現在の Version を取得する
7. Lambda AppSpec を生成する
8. aws deploy create-deployment
9. CodeDeploy の成功を待つ
10. live Alias が新 Version を指していることを確認する
11. 全対象が成功したら last-successful-commit を更新する
```

## 補足

方式比較とトレードオフは [デプロイ戦略の技術調査](./deployment-strategy-research.md) に置く。
