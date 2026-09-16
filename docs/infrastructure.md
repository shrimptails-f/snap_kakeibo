# インフラ設計

## 技術スタック

* AWS CDK for Go
* S3
* API Gateway
* Lambda
* DynamoDB
* Amazon Textract
* OpenAI API
* SNS
* SQS
* CloudWatch Alarm
* SSM Parameter Store

---

## 最終構成

```text
React
  |
  v
API Gateway
  |
  +--> Login Lambda
  |      |
  |      v
  |   users
  |
  +--> Upload Lambda
  |      |
  |      +--> upload_histories
  |      |
  |      +--> Presigned URL
  |
  +--> Retry Lambda
  |      |
  |      +--> upload_histories Update
  |      |
  |      +--> StartExpenseAnalysis
  |
  +--> Recalculate Lambda
  |      |
  |      +--> monthly_summaries Update
  |
  v
S3
  |
  v
StartTextract Queue (SQS) ----> StartTextract DLQ
  |
  v
StartTextract Lambda
  |
  v
Textract
  |
  v
SNS
  |
  v
ResultHandler Queue (SQS) ----> ResultHandler DLQ
  |
  v
ResultHandler Lambda
  |
  +--> OpenAI API
  |      |
  |      +--> 明細カテゴリ分類
  |
  +--> upload_histories Update
  |
  +--> billings Create
  |
  +--> billing_details Create
  |
  +--> monthly_summaries Update

各 DLQ
  |
  v
CloudWatch Alarm
  |
  v
SNS (通知用トピック)
  |
  v
メール
```

---

## S3

### Object Key

```text
receipts/{user_id}/{upload_id}/original.jpg
```

例:

```text
receipts/01JUSERXXX/01JUPLOADXXX/original.jpg
```

S3キーはクライアントから指定させず、Lambda側で生成する。

---

## S3イベント

画像アップロード完了後、S3 ObjectCreatedイベントをSQSへ送り、StartTextract Lambdaを起動する。

```text
S3
  |
  | ObjectCreated
  v
StartTextract Queue (SQS)
  |
  v
StartTextract Lambda
```

S3イベント通知はFIFOキューに送れないため、標準キューを使う。重複配信はLambda側の冪等性で吸収する。

---

## Textract

レシート・請求書解析には以下を使用する。

```text
StartExpenseAnalysis
```

処理フロー:

```text
S3
 |
 v
SQS
 |
 v
Lambda
 |
 | StartExpenseAnalysis
 | DocumentLocation.S3Object.Bucket = 画像保存S3バケット
 | DocumentLocation.S3Object.Name = receipts/{user_id}/{upload_id}/original.jpg
 | ClientRequestToken = {upload_id}#{attempt}
 | JobTag = {user_id}#{upload_id}#{attempt}
 v
Textract
 |
 v
SNS
 |
 v
SQS
 |
 v
Lambda
  |
  | GetExpenseAnalysis
  | OpenAI APIで明細カテゴリ分類
  v
DynamoDB
```

S3からTextractを直接起動するのではなく、LambdaからStartExpenseAnalysisを実行する。

TextractにはS3のバケット名とオブジェクトキーを渡す。アップロード済み画像は `DocumentLocation.S3Object.Bucket` と `DocumentLocation.S3Object.Name` で指定する。

`ClientRequestToken` に `upload_id` と `attempt` を含めることで、同一画像・同一試行に対する重複起動では同じJobIdが返り、二重解析にならない。再実行時は `attempt` が変わるため新しいジョブが起動する。

Textractジョブが `FAILED` または `PARTIAL_SUCCESS` で完了した場合、ResultHandler Lambda は初回を含め最大3回まで自動再実行する。3回使い切った後も、手動再実行APIによる再実行は上限なしで許可する。

`JobTag` には `user_id`、`upload_id`、`attempt` を含める。ResultHandler Lambda は完了通知の `JobTag` から対象の履歴と試行回数を復元し、`status = ANALYZING AND attempt = :attempt` の条件で更新する。古いジョブの遅延通知や重複通知は条件失敗として正常終了する。

Textractの解析結果は完了後7日間 `GetExpenseAnalysis` で取得できる。

---

## OpenAI API

Textract解析後の明細カテゴリ分類に使用する。

呼び出し元は `ResultHandler Lambda` とし、DynamoDB登録前にOpenAI APIを呼び出す。別Lambdaには分けない。

```text
ResultHandler Lambda
  |
  | GetExpenseAnalysis
  v
Textract結果を正規化
  |
  | Responses API
  | Structured Outputs(JSON Schema)
  v
カテゴリ分類結果
  |
  v
DynamoDB TransactWriteItems
```

OpenAI APIの失敗はアップロード全体の失敗にしない。分類できなかった明細は `category = unknown` として登録し、SQSリトライでOpenAI APIだけを再試行する設計にはしない。

ResultHandler Lambdaのタイムアウトは15分とする。

OpenAI API呼び出しは、初期構成では合計60秒を上限にする。60秒の中でエクスポネンシャルバックオフ付きリトライを行う。

APIキーとモデル名はSSM Parameter Storeから取得する。

```text
/app/openai/api-key
/app/openai/model
```

APIキーはSecureString(Standard)、モデル名はStringとして保存する。

---

## SNS

Textractの非同期解析完了通知をSNSで受け、SQSへ送る。

```text
Textract
  |
  | 完了通知
  v
SNS
  |
  v
ResultHandler Queue (SQS)
  |
  v
ResultHandler Lambda
```

Textractの通知先は標準SNSトピックのみのため、後段も標準キューを使う。

---

## SQS

非同期処理のバッファとDLQとして使う。オーケストレーションには使わない。

### キュー

| キュー | 起動元 | 起動先 | DLQ |
| --- | --- | --- | --- |
| StartTextract Queue | S3 ObjectCreated | StartTextract Lambda | StartTextract DLQ |
| ResultHandler Queue | SNS (Textract完了通知) | ResultHandler Lambda | ResultHandler DLQ |

### SQSを挟む理由

```text
リトライ回数と間隔を maxReceiveCount と可視性タイムアウトで制御できる
  (Lambda非同期呼び出しは2回固定、超過後はイベント破棄)

失敗イベントがDLQに残り、redriveで再処理できる
  (追加コード不要、Lambdaが冪等なので戻すだけで安全)

Lambdaの同時実行数を絞ることでTextractの同時ジョブ数を抑えられる
  (一括アップロード時のスロットリング対策)

Lambda自体が停止していてもメッセージがキューに残る
```

API Gateway経由の同期APIには挟まない。ユーザーが結果を待つ処理なので、失敗はそのままエラーレスポンスで返す。

### 設定値

| 項目 | 値 | 理由 |
| --- | --- | --- |
| バッチサイズ | 1 | 1メッセージ = 1画像にし、部分失敗の扱いを不要にする |
| 可視性タイムアウト | Lambdaタイムアウトの6倍 | AWS推奨。Lambda 60秒なら360秒 |
| maxReceiveCount | 3〜5 | 一時エラーの再試行として十分。それ以上は永久失敗の可能性が高い |
| DLQ保持期間 | 14日(最大) | 気づくまでの猶予を最大に取る |
| StartTextract Lambda同時実行数 | 5程度 | Textractの同時ジョブ数を抑える |

### Lambdaの戻り値

SQS起動のLambdaでは「正常終了 = メッセージ削除」「エラー = キューに戻りリトライ」となる。

```text
正常終了(メッセージ削除)
  処理成功
  冪等スキップ
  永久失敗を upload_histories に FAILED として記録済み

エラー(SQSリトライ)
  一時失敗(Throttling、TransactionConflict、ネットワークエラー)
  想定外の例外
```

永久失敗をエラーで返すと無駄なリトライの末にDLQへ行くだけになるため、`FAILED` を記録したら正常終了させる。

### 注意点

```text
SQS標準キューも at-least-once
  ClientRequestToken と status 条件による冪等性は引き続き必要

Textractの結果は7日で消える
  ResultHandler DLQ を8日後に redrive しても GetExpenseAnalysis は失敗する
  → 再実行APIで StartTextract からやり直す

DLQで拾えるのはLambdaが失敗したケースのみ
  イベント自体が届かない、ユーザーがPUTしない、は拾えない
  → 画面側の停滞表示で気づき、手動で再実行する
```

---

## CloudWatch Alarm

各DLQの `ApproximateNumberOfMessagesVisible > 0` を監視し、通知用SNSトピック経由でメール通知する。

```text
StartTextract DLQ ----+
                      |
ResultHandler DLQ ----+--> CloudWatch Alarm --> SNS --> メール
```

### コスト

```text
SQS               月100万リクエストまで恒久無料
CloudWatch Alarm  10個まで無料
SNS               メール通知は月1,000件まで無料
```

---

## SSM Parameter Store

認証用Pepper、JWT署名鍵、OpenAI APIキーをSecureStringとして保存する。

OpenAIのモデル名は通常のStringとして保存する。

```text
SecureString
  /app/auth/password-pepper
  /app/auth/jwt-secret
  /app/openai/api-key

String
  /app/openai/model
```

ティアはStandardを使う(4KB以内、無料)。Secrets Managerは固定費がかかるため使わない。

パラメータの値はCDKで作らず、手動で投入する。CDKはパラメータ名を環境変数としてLambdaへ渡し、Lambdaに `ssm:GetParameter` の権限を付ける。

---

## 採用しないもの

初期構成では以下は使用しない。

```text
Cognito
Step Functions
RDS
ECS
EventBridge Scheduler
```

理由:

```text
個人利用ではオーバースペック

Lambda + DynamoDB中心の構成で十分

固定費を極力持たない
```

SQSはオーケストレーションには使わないが、非同期処理のバッファとDLQとして使う。

停滞検知の定期実行(EventBridge Scheduler)は持たず、画面側の経過時間判定と手動再実行で対応する。
