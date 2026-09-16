# バックエンド設計

## 技術スタック

* Go
* AWS Lambda
* Amazon API Gateway
* DynamoDB
* Amazon Textract
* OpenAI Responses API
* S3 Presigned URL
* SNS
* SQS
* SSM Parameter Store

---

## API

```text
POST /auth/login

POST /uploads

POST /uploads/{upload_id}/retry

GET /monthly-summaries

POST /monthly-summaries/{yyyy-MM}/recalculate

GET /months/{yyyy-MM}/expenses

GET /months/{yyyy-MM}/uploads

GET /billings/{billing_id}

PATCH /billings/{billing_id}
```

画面ごとの利用APIは `docs/screens` 配下に置く。

---

## データの考え方

画像、請求、購入明細、月次集計は分けて保持する。

```text
画像1枚
  |
  v
upload_histories
  |
  | Textract解析成功後
  v
billings
  |
  v
billing_details
```

請求単位では、あとから手動値引きや割り勘調整が入ることを想定する。

```text
billings.original_amount
  -
billings.discount_amount
  =
billings.final_amount
```

月次集計には `final_amount` を反映する。

---

## 画像アップロードフロー

ユーザーは複数画像を一括でアップロードできる。

Textract解析は画像1枚単位で実行する。

```text
1. React
   POST /uploads
   複数画像のファイル名、Content-Typeを送る

2. Lambda
   画像ごとにupload_idを生成

3. DynamoDB
   upload_historiesレコードを画像ごとに作成
   status = UPLOADING
   attempt = 1
   expires_at = Presigned URLの有効期限

4. Lambda
   S3 Presigned PUT URLを画像ごとに発行

5. React
   Presigned URLへ画像ごとにPUT

6. S3
   ObjectCreatedイベント発火
   StartTextract Queue (SQS) へ送信

7. StartTextract Lambda
   画像1枚に対してStartExpenseAnalysis実行
   DocumentLocation.S3Object.Bucket = 画像保存S3バケット
   DocumentLocation.S3Object.Name = upload_histories.s3_key
   ClientRequestToken = {upload_id}#{attempt}
   JobTag = {user_id}#{upload_id}#{attempt}

8. StartTextract Lambda
   upload_histories.status = ANALYZING
   textract_job_idを保存
   Condition: attempt = :attempt

9. Textract
   非同期解析

10. Textract
    SNSへ完了通知
    ResultHandler Queue (SQS) へ送信

11. ResultHandler Lambda
   通知のStatusを確認
   FAILED / PARTIAL_SUCCESSならattemptに応じて自動再実行またはFAILEDにして正常終了
   SUCCEEDEDならGetExpenseAnalysis実行
   JobTagからuser_id / upload_id / attemptを復元
   更新条件は status = ANALYZING AND attempt = :attempt

12. ResultHandler Lambda
    解析結果を検証
    合計金額なし / 購入日なし / 明細50件超
    いずれかに該当すればupload_historiesをFAILEDにして正常終了
    明細0件ならstatus = NO_DATAにして正常終了

13. ResultHandler Lambda
    OpenAI Responses APIで明細のカテゴリ分類を行う
    JSON Schemaで構造化された分類結果を受け取る
    AI分類に失敗した明細はcategory = unknownとして扱う

14. DynamoDB
    TransactWriteItems
    upload_histories更新
    billings作成
    billing_details作成
    monthly_summaries更新

15. status = SUCCEEDED
```

「解析開始(s3_key → JobId)」「AI後処理(Textract結果 → カテゴリ分類)」「結果登録(job_id → DB)」はLambdaハンドラから分離した関数にする。解析開始と結果登録は、SQS経由でも再実行API経由でも同じ関数を呼ぶ。

---

## AI後処理

Textractで取得した店舗名、購入日、合計金額、明細を元に、OpenAI Responses APIで明細カテゴリを分類する。

AI後処理は `ResultHandler Lambda` の中で、DynamoDB登録前に実行する。別Lambdaには分けない。

理由:

```text
カテゴリは billing_details 作成時に必要な値

billings は作成済みだがカテゴリだけ未反映、という中間状態を作らない

初期構成では状態数とLambda数を増やさない
```

AIには金額や日付の正本を決めさせない。月次集計に使う金額はTextract結果から検証した `billings.final_amount` を使い、AIは明細名の補正とカテゴリ分類に限定する。

### 入力

```json
{
  "store_name": "スーパー",
  "purchased_at": "2026-09-15",
  "details": [
    {
      "detail_id": "01JITEMXXX",
      "name": "牛乳",
      "amount": 281
    }
  ]
}
```

### 出力

OpenAI Responses APIのStructured Outputs(JSON Schema)を使い、自由文ではなく固定形式のJSONで受け取る。

```json
{
  "details": [
    {
      "detail_id": "01JITEMXXX",
      "normalized_name": "牛乳",
      "category": "food",
      "confidence": "high"
    }
  ]
}
```

### カテゴリ

初期構成では以下の固定カテゴリに分類する。

```text
food
daily_goods
medical
transport
utilities
entertainment
clothing
education
other
unknown
```

AIが分類できない、レスポンス形式が不正、OpenAI APIが一時失敗した、のいずれかの場合もアップロード全体は `FAILED` にしない。該当明細は `category = unknown`、`category_source = UNKNOWN` として登録する。

AI分類に成功した明細は `category_source = AI` とする。ユーザーが請求詳細・編集画面でカテゴリを修正した場合は `category_source = USER`、`is_edited = true` とする。

### タイムアウトとリトライ

ResultHandler Lambdaのタイムアウトは15分とする。

OpenAI API呼び出しは、初期構成では合計60秒を上限にする。60秒の中でエクスポネンシャルバックオフ付きリトライを行う。

```text
OpenAI API timeout budget = 60秒

retry:
  429 / 500 / 502 / 503 / 504 / ネットワーク一時エラー

no retry:
  400系の恒久エラー
  JSON Schema不一致
```

60秒以内に成功しない場合、アップロード全体は `FAILED` にしない。AI分類失敗として扱い、対象明細は `category = unknown`、`category_source = UNKNOWN` で登録する。

---

## アップロードステータス

```text
UPLOADING
ANALYZING
SUCCEEDED
NO_DATA
FAILED
```

意味:

```text
UPLOADING
Presigned URL発行済み、画像アップロード待ち

ANALYZING
S3アップロード完了、Textract解析中

SUCCEEDED
Textract解析およびDynamoDB登録成功

NO_DATA
Textract解析は完了したが、登録対象の明細が0件
billings / billing_details / monthly_summaries は作成・更新しない

FAILED
Textract解析または登録処理失敗
error_codeに理由を持つ
```

状態遷移:

```text
              Presigned URL発行
                     |
                     v
                UPLOADING ・・・ expires_at 超過は画面側で「期限切れ」表示
                     |
                     | S3 ObjectCreated
                     | StartExpenseAnalysis 成功後に更新
                     | Condition: attempt = :attempt
                     v
                ANALYZING <-------------------------------+
                  |   |                                   |
  Textract成功   |   | 明細0件       Textract失敗       |
  DB登録成功     |   |               内容不十分         |
  Condition:     |   |               明細上限超過       |
  status =       |   |               永久失敗           |
  ANALYZING AND  |   |               Condition:         |
  attempt =      |   |               status = ANALYZING |
  :attempt       |   |               AND attempt =      |
                 v   v               :attempt            |
          SUCCEEDED  NO_DATA          FAILED <------------+
                                      + error_code
                                      + error_message
                                      + failed_at

ANALYZING の停滞は画面側で「停滞」表示 + 再実行ボタン
```

`UPLOADING` の期限切れや `ANALYZING` の停滞は、バックエンドでは状態を変更しない。画面側が `expires_at` や `updated_at` からの経過時間で表示を切り替える。

### error_code

```text
TEXTRACT_FAILED     Textractの起動または解析が失敗した
TEXTRACT_PARTIAL_SUCCESS Textractの解析が部分成功で完了した
NO_TOTAL_AMOUNT     合計金額が取得できなかった
NO_DATE             購入日が取得できなかった
TOO_MANY_DETAILS    明細件数が50件を超えた
INTERNAL            上記以外のシステムエラー
```

`FAILED` の場合、`billings` と `billing_details` は作成しない。`upload_histories` のみ理由付きで残し、履歴画面から参照できる。

`NO_DATA` の場合も `billings` と `billing_details` は作成せず、月次集計にも反映しない。解析自体は完了しているため `FAILED` とは分けて表示する。

---

## 再実行

```text
POST /uploads/{upload_id}/retry
```

`FAILED`、`NO_DATA`、または停滞した `ANALYZING` の画像に対して、解析を最初からやり直す。

処理:

```text
1. upload_histories更新
   Condition: status IN (FAILED, NO_DATA, ANALYZING)
   status = ANALYZING
   attempt = attempt + 1
   textract_job_id / error_code / error_message / failed_at を削除

2. StartExpenseAnalysis実行
   ClientRequestToken = {upload_id}#{attempt}
   JobTag = {user_id}#{upload_id}#{attempt}
   textract_job_idを保存
```

再実行APIで `attempt` を進めた後、解析開始の共通関数を呼ぶ。共通関数は `StartExpenseAnalysis` を先に実行し、成功後に `textract_job_id` を保存する。

`attempt` を `ClientRequestToken` と `JobTag` に含めることで、前回失敗したJobIdではなく新しいジョブが起動し、古いジョブの遅延通知はResultHandlerの条件で捨てられる。

### 自動再実行

Textractジョブの完了通知が `FAILED` または `PARTIAL_SUCCESS` の場合、ResultHandler Lambda は自動再実行を試みる。

```text
対象:
  JobStatus = FAILED
  JobStatus = PARTIAL_SUCCESS

上限:
  attempt <= 3
  初回1回 + 自動リトライ2回

処理:
  attempt < 3:
    attempt = attempt + 1
    error_code / error_message / failed_at を削除
    StartExpenseAnalysis を再実行

  attempt >= 3:
    status = FAILED
    error_code = TEXTRACT_FAILED または TEXTRACT_PARTIAL_SUCCESS
    error_message / failed_at を保存
```

自動再実行は最大3回までとする。3回使い切った後も、ユーザーによる手動再実行は上限なしで許可する。手動再実行でも `attempt` は増やし、`ClientRequestToken` と `JobTag` に新しい `attempt` を含める。

自動再実行はTextract完了通知を処理するResultHandler Lambda内で行う。`FAILED` / `PARTIAL_SUCCESS` の通知では `GetExpenseAnalysis` を呼ばず、`attempt` の更新と `StartExpenseAnalysis` の再実行だけを行う。

Response:

```json
{
  "upload_id": "01JUPLOADXXX",
  "status": "ANALYZING",
  "attempt": 2
}
```

---

## 認証

Cognitoは使用しない。

個人利用のためユーザー登録APIも作成せず、ユーザーは手動登録する。

### パスワード保存

パスワードは暗号化ではなくハッシュ化する。

```text
Password
  +
Random Salt
  +
Pepper
  |
  v
Argon2id
```

保存:

```text
DynamoDB

password_hash
```

SaltはArgon2idのHash文字列に含める。

PepperはSSM Parameter Storeに保存する。

```text
/app/auth/password-pepper
```

---

## ログイン

```text
POST /auth/login
```

Request:

```json
{
  "email": "user@example.com",
  "password": "password"
}
```

処理:

```text
DynamoDBからユーザー取得

↓

SSMからPepper取得

↓

Argon2id Verify

↓

JWT発行
```

以降のAPI:

```http
Authorization: Bearer {JWT}
```

---

## DynamoDB

DB設計の詳細は [DB設計](./database.md) に置く。

バックエンド処理で扱うテーブルは以下とする。

```text
users

monthly_summaries

upload_histories

billings

billing_details
```

---

## 月次集計

月次集計はVersionによるRead-Modify-Writeではなく、DynamoDBの原子的更新を使用する。

```text
ADD total_amount :final_amount
ADD billing_count :one
ADD detail_count :detail_count
```

ただしLambda再実行による二重計上を防止するため、billings作成、billing_details作成、upload_histories更新、monthly_summaries更新をTransactWriteItemsで行う。

```text
Transaction

1. upload_histories

Condition:
status = ANALYZING AND attempt = :attempt

Update:
status = SUCCEEDED
billing_id = 01JBILLXXX

2. billings

Condition:
attribute_not_exists(PK)

Put:
billings

3. billing_details (最大50件)

Condition:
attribute_not_exists(PK)

Put:
billing_details

4. monthly_summaries

SET:
type = if_not_exists(type, :type)
user_id = if_not_exists(user_id, :user_id)
year_month = if_not_exists(year_month, :year_month)
category_totals = 更新後のカテゴリ別合計

ADD:
total_amount + final_amount
billing_count + 1
detail_count + detail_count
version + 1
```

これにより同一イベントが複数回処理されても月次集計への二重加算を防止する。

TransactWriteItemsは100 itemまでのため、明細は50件を上限とする(Transaction内53 item)。これは初期構成のプロダクト仕様とし、超過した場合は `TOO_MANY_DETAILS` として `FAILED` にし、billingsとbilling_detailsは作成しない。

### TransactionCanceledExceptionの分類

`CancellationReasons` を見て処理を分ける。

```text
ConditionalCheckFailed
  冪等スキップ。正常終了(SQSから削除)

TransactionConflict / Throttling
  一時失敗。エラーを返してSQSリトライ

ValidationError
  永久失敗。FAILED + error_code = INTERNAL を記録して正常終了
```

---

## 月次再計算

```text
POST /monthly-summaries/{yyyy-MM}/recalculate
```

指定月の `monthly_summaries` を元データから作り直す。集計がズレたときの復旧と、請求編集時の反映に使う。

処理:

```text
1. monthly_summariesを取得し、versionを控える
   レコードが存在しない場合は version = 0 とみなす

2. billingsを取得
   PK = USER#{user_id} and SK begins_with BILLING#
   year_monthでフィルタ

3. billing_detailsを取得
   detail_month_amount_index
   GSI1PK = USER#{user_id}#MONTH#{yyyy-MM}

4. 集計値を計算
   total_amount    = SUM(billings.final_amount)
   billing_count   = COUNT(billings)
   detail_count    = COUNT(billing_details)
   category_totals = SUM(billing_details.amount) GROUP BY category

5. monthly_summaries更新
   Condition: attribute_not_exists(PK) OR version = :v
   SET total_amount, billing_count, detail_count, category_totals
   SET type / user_id / year_month
   SET version = if_not_exists(version, 0) + 1
```

条件失敗した場合(再計算中にResultHandlerが `ADD` した場合)は 1 からやり直す。数回リトライして諦める。個人利用では実質起きない。

月次集計は `user_id + year_month` で一意になるため、初回作成判定用のUUIDは持たない。UUIDを別カラムに追加しても、再計算時の競合制御には使えないため、初回作成は `attribute_not_exists(PK)` と `if_not_exists` で扱う。

---

## 請求編集

請求詳細画面では、請求単位の値引き額をあとから変更できる。

```text
PATCH /billings/{billing_id}
```

編集対象:

```text
store_name
purchased_at
discount_amount
billing_details.name
billing_details.amount
billing_details.quantity
billing_details.category
```

処理:

```text
1. billings / billing_detailsを更新
   final_amount = original_amount - discount_amount
   is_edited = true
   カテゴリを変更した明細は category_source = USER
   billing_detailsのGSIキーも更新

2. 月次再計算を実行
   purchased_atの月が変わる場合は旧月と新月の両方
```

差分更新(`ADD total_amount :diff_amount`)は行わない。月あたりの請求数は数十件のため、再計算のコストは無視できる。差分計算の競合や月またぎ・カテゴリ変更の複雑さを避ける。

---

## GET /months/{yyyy-MM}/expenses

指定月の購入明細を金額降順で取得する。

表示上限とページネーションは初期構成では持たない。

```text
detail_month_amount_index

GSI1PK = USER#{user_id}#MONTH#{yyyy-MM}
```

---

## GET /months/{yyyy-MM}/uploads

指定月の画像アップロード履歴を取得する。

```text
upload_month_index

GSI1PK = USER#{user_id}#MONTH#{yyyy-MM}
```

---

## エラーハンドリング

### 方針

```text
永久失敗は upload_histories に FAILED + error_code で残し、正常終了する
一時失敗はエラーを返し、SQSのリトライに任せる
リトライ枯渇はDLQに残り、CloudWatch Alarmで通知する
停滞は画面側で検知し、再実行APIで手動回復する
```

### 失敗ケース一覧

| 段階 | ケース | 対応 |
| --- | --- | --- |
| アップロード | Presigned URL発行後にPUTされない | `UPLOADING` のまま残す。画面が `expires_at` 超過で「期限切れ」表示 |
| 解析開始 | S3イベント重複 | `ClientRequestToken` で同一JobId。`Condition: attempt = :attempt` で現在の試行だけを更新 |
| 解析開始 | StartExpenseAnalysisが同期エラー | ValidationException系は `FAILED` (`TEXTRACT_FAILED`)。Throttlingはエラーを返してSQSリトライ |
| 解析完了 | TextractジョブがFAILED | `attempt < 3` なら自動再実行。`attempt >= 3` なら `FAILED` (`TEXTRACT_FAILED`) |
| 解析完了 | TextractジョブがPARTIAL_SUCCESS | `attempt < 3` なら自動再実行。`attempt >= 3` なら `FAILED` (`TEXTRACT_PARTIAL_SUCCESS`) |
| 解析完了 | 合計金額 / 購入日が取れない | `FAILED` (`NO_TOTAL_AMOUNT` / `NO_DATE`)。billingsは作らない |
| 解析完了 | 明細0件 | `NO_DATA`。解析は完了したが登録対象なしとして扱い、billingsと月次集計は作らない |
| AI後処理 | OpenAI API呼び出し失敗 / JSON不正 | アップロード全体は失敗にしない。対象明細は `category = unknown` で登録 |
| 登録 | 明細50件超 | `FAILED` (`TOO_MANY_DETAILS`)。初期構成の仕様上、billingsは作らない |
| 登録 | 古いTextract通知 / SNS通知重複 | `status = ANALYZING AND attempt = :attempt` で弾く。`ConditionalCheckFailed` は正常終了 |
| 登録 | ResultHandlerが途中で落ちる | Transactionはall-or-nothing、S3保存は上書き冪等。SQSリトライで最初からやり直す |
| 登録 | リトライ枯渇 | DLQに残る。CloudWatch Alarmでメール通知。redriveで再処理 |
| 登録 | Textract結果の7日期限を過ぎた | redriveでは復旧できない。再実行APIでStartTextractからやり直す |
| 検知 | イベント自体が届かない | DLQでは拾えない。画面の停滞表示で気づき、再実行APIで手動回復 |
| 集計 | 再計算とResultHandlerの競合 | `version` の条件失敗でやり直す |

### 冪等性

二重実行されても以下が二重登録されないことを保証する。

```text
billings
billing_details
monthly_summaries
```

担保する仕組み:

```text
StartTextract    Condition: attempt = :attempt
                 ClientRequestToken = {upload_id}#{attempt}
                 JobTag = {user_id}#{upload_id}#{attempt}

ResultHandler    Condition: status = ANALYZING AND attempt = :attempt
                 TransactWriteItems

再計算           Condition: attribute_not_exists(PK) OR version = :v
```

### 許容するリスク

個人利用のため、以下は対応せず運用で許容する。

```text
イベント未達(S3 / SNS)の自動回復
  → 画面で気づいて手動再実行

再計算とResultHandlerの競合が連続する
  → version条件失敗のリトライ数回で諦める。実質起きない

Textract結果の7日期限を過ぎたDLQメッセージ
  → 再実行APIで最初からやり直す
```
