# デプロイ戦略の技術調査

## 調査対象

API Gateway 経由の Lambda を安全に入れ替える方式を比較する。

現在の構成は次のとおり。

- API Gateway は HTTP API(v2)
- Lambda は関数ごとの ECR コンテナイメージ
- ECR タグは git SHA
- 関数ごとに SSM Parameter Store の image-tag を持つ
- API Gateway は Lambda 関数本体を直接呼ぶ

## 比較

| 方式 | 概要 | 良い点 | 注意点 |
| --- | --- | --- | --- |
| 直更新 | Lambda 関数本体を新しいイメージへ更新する | 構成が最小。理解しやすい | 段階デプロイ不可。戻すには前タグへの更新が必要 |
| SSM image-tag + CDK deploy | SSM の image-tag を更新し、CDK deploy で Lambda を更新する | 現在の構成に近い。CDK で一貫管理できる | コード更新でも CloudFormation deploy が走る。段階デプロイ不可 |
| Version + Alias All-at-once | 呼び出し先を `live` Alias にし、新 Version へ一気に切り替える | 旧 Version が残り、ロールバックしやすい | 段階デプロイはしない。Alias と Version 管理が必要 |
| Version + Alias weighted routing | Alias が旧 Version と新 Version に割合で振り分ける | 少量 traffic で新 Version を試せる | 新旧 Version のデータ互換性が必要。低 traffic では割合がぶれやすい |
| CodeDeploy Lambda Canary / Linear | Alias の traffic shifting を CodeDeploy に任せる | 履歴、段階移行、Alarm rollback を扱える | CodeDeploy 構成、AppSpec、監視設計が必要 |
| API Gateway REST API Canary | API Gateway stage 側で canary を扱う | REST API の stage 運用と相性がよい | HTTP API(v2) では同じ前提で使えない |
| 関数名 Blue/Green | 旧関数と新関数を別 Lambda として作り、API 統合先を切り替える | 設定や権限まで大きく変えやすい | 関数数、IAM、ログ、API 統合が複雑になる |

## 直更新

```text
API Gateway
  |
  v
Lambda function
  old image -> new image
```

最も単純な方式。API Gateway 側は変えず、Lambda 関数本体のコードを更新する。

dev や小さい個人アプリでは扱いやすいが、新旧の traffic split はできない。デプロイ直後の全 traffic が新コードへ向かう。

## SSM image-tag + CDK deploy

```text
ECR
  {function}:{git sha}

SSM
  /{stage}/snap-kakeibo/functions/{function}/image-tag = {git sha}

CDK deploy app
  SSM image-tag を解決して Lambda を更新
```

成果物の配置を CDK の外で行い、App スタックは SSM の image-tag を参照して Lambda を更新する。

関数ごとの ECR repository と git SHA tag を保てる一方、Lambda Version / Alias を使わない限り段階デプロイはできない。関数コードだけの更新でも CloudFormation deploy が走る。

## Version + Alias All-at-once

```text
API Gateway
  |
  v
Lambda alias: live
  |
  v
Version 12

deploy

Lambda alias: live
  |
  v
Version 13
```

API Gateway の呼び先を Lambda 関数本体ではなく `live` Alias にする。デプロイ時に新しい Version を publish し、Alias を切り替える。

旧 Version が残るため、Alias を戻せばロールバックできる。Canary / Linear へ発展しやすい。

## Version + Alias weighted routing

```text
API Gateway
  |
  v
Lambda alias: live
  |-- Version 12: 90%
  `-- Version 13: 10%
```

Lambda Alias の routing configuration で、同じ関数の published Version 2つに traffic を分ける。

少量の traffic で新 Version を試せる。API Gateway 側の設定変更は不要。

一方で、新旧 Version が同じ DynamoDB / S3 / SQS を触るため、データ形式と副作用の後方互換性が必要になる。同じユーザーが旧新どちらにも当たる可能性もある。

## CodeDeploy Lambda Canary / Linear

```text
CodeDeploy
  |
  v
Lambda alias: live
  |-- old version
  `-- new version
```

CodeDeploy が Alias の traffic shifting を管理する。

| 設定例 | 動き |
| --- | --- |
| `LambdaAllAtOnce` | すべての traffic を一度に新 Version へ流す |
| `LambdaCanary10Percent5Minutes` | まず 10%、5分後に残り 90% |
| `LambdaCanary10Percent10Minutes` | まず 10%、10分後に残り 90% |
| `LambdaLinear10PercentEvery1Minute` | 1分ごとに 10% ずつ増やす |
| `LambdaLinear10PercentEvery2Minutes` | 2分ごとに 10% ずつ増やす |

CodeDeploy を使うと、デプロイ履歴、段階移行、CloudWatch Alarm による rollback、PreTraffic / PostTraffic hook を扱える。

注意点は、CodeDeploy Application / Deployment Group / Service Role、AppSpec、監視指標と Alarm の設計が必要になること。

CodeDeploy の Canary / Linear の間隔は分単位で扱う。CloudFormation の `TimeBasedCanary.CanaryInterval` も分数を指定するため、秒単位の 5秒 / 10秒 canary は標準機能では表現しない。

## API Gateway REST API Canary

API Gateway REST API には stage variables や canary release の仕組みがある。Stage variable を Lambda function name、version、alias の一部として使える。

```text
REST API stage
  lambdaAlias = live

Integration URI
  function:upload:${stageVariables.lambdaAlias}
```

REST API の stage 運用と相性がよいが、現在の構成は HTTP API(v2) なので第一候補にはしない。Lambda だけを段階移行したい場合は Alias / CodeDeploy の方が素直。

## 関数名 Blue/Green

```text
API Gateway
  |
  v
upload-blue

deploy

API Gateway
  |
  v
upload-green
```

旧関数と新関数を別 Lambda として作り、API Gateway integration を切り替える。

Lambda 設定、環境変数、IAM、event source mapping を大きく変える移行では使える。ただし、関数数、ECR、ロググループ、IAM、API Gateway の切り替えが複雑になる。

## SQS 起動 Lambda

SQS event source mapping でも Alias をターゲットにできる。

```text
SQS
  |
  v
Lambda alias: live
  |-- old version
  `-- new version
```

ただし、HTTP API と違ってユーザーリクエスト単位の体感ではなく、キュー上のメッセージ処理が新旧に分かれる。

SQS 起動 Lambda で Canary / Linear を使う場合は、次が必要になる。

- 同じメッセージ形式を新旧両方が処理できる
- DynamoDB 更新条件が新旧で競合しない
- visibility timeout と retry が想定どおり動く
- DLQ alarm で悪化を検知できる

`analyze-receipt` は処理が重く、OpenAI 呼び出しのばらつきもあるため、traffic percentage よりも失敗率、DLQ流入、処理時間、コストの監視が重要。

## 参考

- AWS Lambda: Implement Lambda canary deployments using a weighted alias  
  https://docs.aws.amazon.com/lambda/latest/dg/configuring-alias-routing.html
- AWS CodeDeploy: Deployment configurations for AWS Lambda  
  https://docs.aws.amazon.com/codedeploy/latest/userguide/deployment-configurations.html
- AWS CloudFormation: AWS::CodeDeploy::DeploymentConfig TimeBasedCanary  
  https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-properties-codedeploy-deploymentconfig-timebasedcanary.html
- Amazon API Gateway: Stage variables reference for REST APIs  
  https://docs.aws.amazon.com/apigateway/latest/developerguide/aws-api-gateway-stage-variables-reference.html
