# バックエンド設計

## 技術スタック

* Go
* AWS Lambda
* Amazon API Gateway
* DynamoDB
* OpenAI Responses API(画像入力 + Structured Outputs)
* S3 Presigned URL
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
  | 解析成功後
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

解析は画像1枚単位で実行する。

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
   Analyze Queue (SQS) へ送信

7. Analyze Lambda
   S3キーからuser_id / upload_idを復元(attempt = 1)
   upload_histories.status = ANALYZING
   Condition: status IN (UPLOADING, ANALYZING) AND attempt = :attempt
   条件失敗は処理済みか古い試行なので正常終了

8. Analyze Lambda
   S3から画像取得、長辺2048pxに縮小、JPEGでBase64
   OpenAI Responses APIに画像と指示文を渡し、JSON Schemaで結果を受け取る
   店名 / 購入日 / 合計金額 / 明細(品目名・金額・数量・カテゴリ)
   生レスポンスを S3 analysis-results/{user_id}/{upload_id}/{attempt}/{response_id}.json に保存

9. Analyze Lambda
   解析結果を検証(「検証」の表)
   合計金額なし・範囲外 / 購入日なし・不正 / 明細50件超
   いずれかに該当すればupload_historiesをFAILEDにして正常終了
   明細0件(レシートでない画像を含む)ならstatus = NO_DATAにして正常終了

10. DynamoDB
    TransactWriteItems
    upload_histories更新(Condition: status = ANALYZING AND attempt = :attempt)
    billings作成
    billing_details作成
    monthly_summaries更新(すべて ADD。map の SET はしない)

11. status = SUCCEEDED
```

「画像取得と縮小」「OpenAI呼び出し」「検証」「結果登録」はLambdaハンドラから分離した関数にする。S3イベント経由でも再実行API経由のメッセージでも、`user_id + upload_id + attempt` に解決したあとは同じ関数を呼ぶ。

### Analyze Queue のメッセージ

```text
S3 イベント通知   S3 のキーから user_id / upload_id を取り出す。attempt = 1 とみなす
再実行 API        {"user_id": "...", "upload_id": "...", "attempt": 2, "trigger": "RETRY"}
```

`attempt` はメッセージ側で固定し、DynamoDB から「現在の attempt」を読んで使うことはしない。遅れて届いた古いメッセージや重複した S3 イベントが、新しい試行として処理されるのを防ぐため。

### 試行の排除

開始・登録・失敗のすべての更新に `attempt = :attempt` を条件に入れる。

```text
開始              Condition: status IN (UPLOADING, ANALYZING) AND attempt = :attempt
登録(SUCCEEDED)   Condition: status = ANALYZING AND attempt = :attempt
FAILED / NO_DATA  Condition: status = ANALYZING AND attempt = :attempt
```

開始条件に `ANALYZING` を含めるのは、SQS の再配信(一時エラーで Lambda がエラー終了した同じメッセージ)を通すため。`status = UPLOADING` だけにすると、2回目以降の配信が弾かれてリトライが機能しない。

`FAILED` / `NO_DATA` にも条件を付けるのは、attempt 1 の遅い失敗処理が attempt 2 の成功結果を上書きしないため。

同じ `attempt` の並行処理(S3 イベントの重複配信)は開始条件を両方通る。OpenAI を二重に呼び、SUCCEEDED / FAILED / NO_DATA を問わず**最初に終端状態を書いた方が勝ち**、後の方は `ConditionalCheckFailed` で正常終了する。片方が正常解析、もう片方が refusal で先に `FAILED` を書くと最終状態は `FAILED` になるが、手動再実行で回復できるため許容する(「許容するリスク」参照)。

---

## レシート解析(OpenAI)

読み取りとカテゴリ分類はOpenAI Responses APIへの1回の呼び出しで行う。OCRサービス(Textract)は日本語非対応のため使わない。

`Analyze Lambda` の中で、DynamoDB登録前に実行する。別Lambdaには分けない。

理由:

```text
カテゴリは billing_details 作成時に必要な値

billings は作成済みだがカテゴリだけ未反映、という中間状態を作らない

初期構成では状態数とLambda数を増やさない
```

### 入力

```text
model                = SSM の値(初期値 gpt-5-mini)
reasoning.effort     = SSM の値(初期値 low)
store                = false(レシートを OpenAI 側に保存させない)
max_output_tokens    = 4096
text.format          = json_schema(strict)

instructions(固定文。cached input を効かせるため毎回同じにする)
  レシート画像から店名・購入日・合計金額・明細を読み取り、明細を固定カテゴリに分類する
  読めない項目は null にする。推測で埋めない
  金額は税込の整数(円)。合計金額はレシートの「合計」「お買上げ計」など支払額の行を使う
  明細は商品行のみ。小計・税・割引・預り金・お釣りの行は明細に含めない
  レシートでない画像なら details を空にする

input
  input_image
    image_url = data:image/jpeg;base64,...(長辺2048pxを上限に縮小したJPEG。拡大はしない)
    detail    = high
```

縮小の上限は環境変数 `IMAGE_MAX_EDGE`(初期値 2048)で変える。細長いレシートは縮小で文字が潰れやすいため、実画像で評価してから値を決める。

### 出力

Structured Outputs(JSON Schema、`strict: true`)で固定形式のJSONで受け取る。

```json
{
  "store_name": "ファミリーマート",
  "purchased_at": "2024-01-05",
  "total_amount": 106,
  "details": [
    {
      "name": "スイートオレンジ&温州",
      "amount": 106,
      "quantity": 1,
      "category": "food"
    }
  ]
}
```

| 項目 | 型 | 備考 |
| --- | --- | --- |
| store_name | string / null | 読めなければ null。null でも登録は続ける |
| purchased_at | string(YYYY-MM-DD) / null | null なら `NO_DATE` |
| total_amount | integer / null | null なら `NO_TOTAL_AMOUNT` |
| details[].name | string | レシート表記のまま |
| details[].amount | integer | 行の金額(数量をかけた後) |
| details[].quantity | integer | 読めなければ 1 |
| details[].category | enum | 下記の固定カテゴリ |

### 検証

JSON Schema に合っていてもレシートとして正しい値とは限らないので、登録前にアプリ側で意味検証する。

| 項目 | 検証 | 違反時 |
| --- | --- | --- |
| purchased_at | null でない | `NO_DATE` |
| purchased_at | `YYYY-MM-DD` として実在する日付(2月30日などを弾く) | `INVALID_DATE` |
| purchased_at | 未来でない(タイムゾーン差を考慮して翌日まで許容) | `INVALID_DATE` |
| purchased_at | 5年より前でない | `INVALID_DATE` |
| total_amount | null でない | `NO_TOTAL_AMOUNT` |
| total_amount | 1 以上 10,000,000 以下 | `INVALID_AMOUNT` |
| details | 50件以下 | `TOO_MANY_DETAILS` |
| details[].amount | 0 以上 10,000,000 以下 | `INVALID_AMOUNT` |
| details[].quantity | 1 以上 999 以下 | `INVALID_AMOUNT` |
| details[].name | 空でない | 該当明細を捨てる(他の明細は登録する) |
| store_name / details[].name | 100 文字を超える分は切り詰める | 失敗にしない |

明細の合計と `total_amount` が一致しなくても `FAILED` にしない。値引きや税の丸めで一致しないことが多く、月次集計には `total_amount` 由来の `billings.final_amount` を使うため明細の誤差は集計に影響しない。

`INVALID_DATE` / `INVALID_AMOUNT` は読み取りミスの可能性が高いので、履歴画面から手動再実行できる。再実行でも直らない場合は画像側の問題(手ブレ、見切れ)として扱う。

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

登録時は `category_source = AI` とする。ユーザーが請求詳細・編集画面でカテゴリを修正した場合は `category_source = USER`、`is_edited = true` とする。

### モデルと reasoning effort

Analyze Lambda の環境変数として設定する。変更時は app スタックを再デプロイする。

```text
OPENAI_MODEL             初期値 gpt-5-mini
OPENAI_REASONING_EFFORT  初期値 low
```

思考トークンは出力トークンとして課金されるため `low` か `minimal` にする。レスポンスの `usage.output_tokens_details.reasoning_tokens` と `usage.input_tokens` をログに出し、実測でどちらにするか決める。

### レスポンスの判定

Structured Outputs でも「JSON が返る」以外の終わり方がある。`status` と `output` を見て分類する。

| レスポンス | 扱い |
| --- | --- |
| `status = completed`、`output_text` あり、JSON Schema でパース成功 | 正常解析 |
| `status = completed` だが `output` に `refusal` | 恒久失敗 `ANALYSIS_FAILED`。`error_message` に refusal の内容 |
| `status = completed` だが期待する output が無い | 恒久失敗 `ANALYSIS_FAILED` |
| `status = incomplete`、`incomplete_details.reason = max_output_tokens` | 恒久失敗 `ANALYSIS_FAILED`(同じ入力で再試行しても同じ結果になる) |
| `status = incomplete`、`reason = content_filter` | 恒久失敗 `ANALYSIS_FAILED` |
| `status = failed` | 一時失敗。リトライ予算内で再試行 |
| HTTP 429 / 5xx / ネットワークエラー | 一時失敗。リトライ予算内で再試行 |
| HTTP 400 系(画像サイズ超過など) | 恒久失敗 `ANALYSIS_FAILED` |
| JSON Schema でパース失敗 | 恒久失敗 `ANALYSIS_FAILED` |

`max_output_tokens` は明細 50 件 + 余裕で足りる値(4,096)にする。思考トークンも `max_output_tokens` に含まれるため、reasoning effort を上げるときは合わせて見直す。

### タイムアウトとリトライ

Analyze Lambdaのタイムアウトは3分とする。

OpenAI API呼び出しは合計120秒を上限にする。120秒の中でエクスポネンシャルバックオフ付きリトライを行う。

```text
OpenAI API timeout budget = 120秒

retry:
  429 / 500 / 502 / 503 / 504 / ネットワーク一時エラー

no retry:
  400系の恒久エラー(画像が大きすぎる、コンテンツ拒否など)
  JSON Schema不一致
```

120秒以内に成功しない場合はエラーを返し、SQSリトライに任せる。恒久エラーは `FAILED`(`ANALYSIS_FAILED`)を記録して正常終了する。

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
S3アップロード完了、解析中

SUCCEEDED
解析およびDynamoDB登録成功

NO_DATA
解析は完了したが、登録対象の明細が0件(レシートでない画像を含む)
billings / billing_details / monthly_summaries は作成・更新しない

FAILED
解析または登録処理失敗
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
                     | Analyze Lambda が処理開始時に更新
                     | Condition: status IN (UPLOADING, ANALYZING)
                     |            AND attempt = :attempt
                     v
                ANALYZING <-------------------------------+
                  |   |                                   |
  解析成功       |   | 明細0件       OpenAI 恒久失敗    |
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
ANALYSIS_FAILED     OpenAI APIが恒久エラーを返した、または応答がJSON Schemaに合わなかった
NO_TOTAL_AMOUNT     合計金額が取得できなかった
INVALID_AMOUNT      合計金額または明細の金額・数量が範囲外
NO_DATE             購入日が取得できなかった
INVALID_DATE        購入日が実在しない、未来、または5年より前
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
   error_code / error_message / failed_at を削除
   ReturnValues: UPDATED_NEW で新しい attempt を受け取る

2. Analyze Queue へ送信
   {"user_id": "...", "upload_id": "...", "attempt": 2, "trigger": "RETRY"}
```

`attempt` を進めることで、停滞していた前回の処理がまだ動いていても、その登録・失敗記録は `attempt = :attempt` の条件失敗で捨てられる。

### 自動再実行

一時エラー(OpenAI の 429 / 5xx、Lambda タイムアウト、DynamoDB スロットリング)は SQS のリトライ(`maxReceiveCount = 3`)で自動再実行する。SQS リトライでは `attempt` を進めない。

恒久エラーは自動再実行せず `FAILED` にする。ユーザーによる手動再実行は上限なしで許可する。

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

### JWT署名鍵

JWTはHS256で署名する。署名鍵はSSM Parameter Store(SecureString、Standard)に保存する。

```text
/app/auth/jwt-secret
```

Lambdaは起動時にSSMから取得し、コンテナが生きている間はメモリに保持する。

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
raw_result_s3_key = analysis-results/{user_id}/{upload_id}/{attempt}/{response_id}.json

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

ADD:
total_amount + final_amount
billing_count + 1
detail_count + detail_count
category_total_{category} + カテゴリ別小計(登場したカテゴリのみ)
version + 1
```

これにより同一イベントが複数回処理されても月次集計への二重加算を防止する。

カテゴリ別金額は map の `SET` ではなく、カテゴリごとのトップレベル数値属性への `ADD` にする。map を読んで計算した値を `SET` すると、同じ月のレシートを並行処理したときに後勝ちでもう一方の加算が消える。DynamoDB は同じ式の中で `SET category_totals = if_not_exists(...)` と `ADD category_totals.food` を書けず(パスの重複)、map が無い状態での nested path への `ADD` も失敗するため、map ではなく `category_total_food` のような属性に分ける。API はこれらを `category_totals` の map に組み立てて返す。

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
   total_amount             = SUM(billings.final_amount)
   billing_count            = COUNT(billings)
   detail_count             = COUNT(billing_details)
   category_total_{category} = SUM(billing_details.amount) GROUP BY category(全カテゴリ。0 も SET)

5. monthly_summaries更新
   Condition: attribute_not_exists(PK) OR version = :v
   SET total_amount, billing_count, detail_count, category_total_{category} × 全カテゴリ
   SET type / user_id / year_month
   SET version = if_not_exists(version, 0) + 1
```

条件失敗した場合(再計算中にAnalyze Lambdaが `ADD` した場合)は 1 からやり直す。数回リトライして諦める。個人利用では実質起きない。

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
| 解析 | S3イベント重複 / 古いメッセージの遅延配信 | 開始条件 `status IN (UPLOADING, ANALYZING) AND attempt = :attempt` で古い試行を弾く。同じ試行の並行は OpenAI を二重に呼ぶが、登録は条件で1回になる |
| 解析 | OpenAI の refusal / incomplete(max_output_tokens, content_filter) | `FAILED` (`ANALYSIS_FAILED`)。`error_message` に理由 |
| 解析 | OpenAI APIの一時エラー(429 / 5xx) | 120秒の予算内でリトライ。尽きたらエラーを返してSQSリトライ |
| 解析 | OpenAI APIの恒久エラー / JSON Schema不一致 | `FAILED` (`ANALYSIS_FAILED`)。手動再実行で回復 |
| 解析 | 合計金額 / 購入日が取れない | `FAILED` (`NO_TOTAL_AMOUNT` / `NO_DATE`)。billingsは作らない |
| 解析 | 日付が実在しない・未来・古すぎる、金額や数量が範囲外 | `FAILED` (`INVALID_DATE` / `INVALID_AMOUNT`)。読み取りミスとして手動再実行 |
| 解析 | 明細0件 / レシートでない画像 | `NO_DATA`。解析は完了したが登録対象なしとして扱い、billingsと月次集計は作らない |
| 登録 | 明細50件超 | `FAILED` (`TOO_MANY_DETAILS`)。初期構成の仕様上、billingsは作らない |
| 登録 | 停滞した前回処理の遅延登録・遅延失敗記録 | SUCCEEDED / FAILED / NO_DATA すべて `status = ANALYZING AND attempt = :attempt` で弾く。`ConditionalCheckFailed` は正常終了 |
| 集計 | 同じ月のレシートの並行登録 | `category_total_*` を含めすべて `ADD` なので競合しない |
| 登録 | Analyze Lambdaが途中で落ちる | Transactionはall-or-nothing。生レスポンスは response_id ごとの一意なS3キーへ保存し、SQSリトライで最初からやり直す |
| 登録 | リトライ枯渇 | DLQに残る。CloudWatch Alarmでメール通知。redriveで再処理 |
| 検知 | イベント自体が届かない | DLQでは拾えない。画面の停滞表示で気づき、再実行APIで手動回復 |
| 集計 | 再計算とAnalyze Lambdaの競合 | `version` の条件失敗でやり直す |

### 冪等性

二重実行されても以下が二重登録されないことを保証する。

```text
billings
billing_details
monthly_summaries
```

担保する仕組み:

```text
Analyze(開始)          Condition: status IN (UPLOADING, ANALYZING) AND attempt = :attempt
                        attempt はメッセージ側の値(S3 イベントは 1)

Analyze(登録)          Condition: status = ANALYZING AND attempt = :attempt
                        TransactWriteItems、月次集計は ADD のみ

Analyze(FAILED/NO_DATA) Condition: status = ANALYZING AND attempt = :attempt

再計算                  Condition: attribute_not_exists(PK) OR version = :v
```

### 許容するリスク

個人利用のため、以下は対応せず運用で許容する。

```text
イベント未達(S3)の自動回復
  → 画面で気づいて手動再実行

再計算とAnalyze Lambdaの競合が連続する
  → version条件失敗のリトライ数回で諦める。実質起きない

S3イベント重複によるOpenAIの二重呼び出し
  → 1枚 ¥1 未満なので許容。登録は条件で1回になる
  → 生レスポンスは response_id ごとの別キーに保存し、終端状態を確定した処理のキーだけを raw_result_s3_key に記録する

同じattemptの並行処理で、成功した方ではなく先に終端状態を書いた方が勝つ
  → 正常解析と refusal が並行し、refusal が先に FAILED を書けば最終状態は FAILED
  → 手動再実行で回復できる。成功を優先するには永久失敗の確定を遅らせる仕組みが要るため、現状規模では持たない
```
