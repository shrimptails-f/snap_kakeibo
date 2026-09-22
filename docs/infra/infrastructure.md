# インフラ設計

## 技術スタック

* AWS CDK for Go
* S3
* CloudFront
* ECR
* API Gateway
* Lambda
* DynamoDB
* OpenAI API(画像入力 + Structured Outputs)
* SNS
* SQS
* CloudWatch Alarm
* SSM Parameter Store

---

## 最終構成

```text
ブラウザ
  |
  v
CloudFront (OAC) ----> S3 (frontend bucket, React ビルド成果物)
  |
  | React から HTTPS
  v
API Gateway
  |
  +--> Login / Refresh / Logout Lambda
  |      |
  |      +--> users
  |      |
  |      +--> refresh_tokens
  |
  +--> Upload Lambda
  |      |
  |      +--> analysis_requests
  |      |
  |      +--> Presigned URL
  |
  +--> Retry Analysis Lambda
  |      |
  |      +--> analysis_requests Update
  |      |
  |      +--> Analyze Queue へ送信
  |
  +--> Rebuild Lambda
  |      |
  |      +--> monthly_summaries Update
  |
  v
S3
  |
  v
Analyze Queue (SQS) ----> Analyze DLQ
  |
  v
Analyze Lambda
  |
  +--> S3 から画像取得
  |
  +--> OpenAI API
  |      |
  |      +--> 店名 / 購入日 / 合計 / 明細 / カテゴリを1回で読み取る
  |
  +--> analysis_requests Update
  |
  +--> expenses Create
  |
  +--> expense_details Create
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

## CDK スタック

失うと困るものと作り直せるものでスタックを分ける。依存は `app -> storage` の一方向のみ。

| スタック | 中身 | 備考 |
| --- | --- | --- |
| `{stage}-snap-kakeibo-storage` | ECR(関数ごと)、S3(receipt / front)、DynamoDB、SQS + DLQ、SNS(アラート)、DLQ アラーム、Lambda のロググループ | 人間が成果物を push する先。stage の `RemovalPolicy` で残すかを決める |
| `{stage}-snap-kakeibo-app` | Lambda、API Gateway、CloudFront、イベントソースマッピング | destroy して作り直せる |

SQS / SNS を storage 側に置くのは、S3 → SQS の通知設定がバケット側のスタックに生成されるため(app 側に置くと循環参照になる)と、DLQ のメッセージとメール購読の確認状態を失いたくないため。

### リージョン

`ap-northeast-2`(ソウル)を使う(`infra/common/const.go` の `AWSRegion`)。当初 Textract を使う予定で、Textract が東京に無かったためソウルにした。Textract を使わなくなった後もリージョンにこだわりが無いため据え置いている。東京に戻す場合は定数を変えて storage から作り直す。

### デプロイ手順

```text
1. task infra:deploy:storage   SSM パラメータ作成 + storage スタック
2. 人間が成果物を push
     Lambda イメージ  -> task image:push(関数ごとに arm64 でビルド、git SHA タグで専用 ECR へ、SSM のタグを更新)
     React ビルド     -> task front:push(S3 sync + CloudFront 無効化)
3. task infra:deploy:app       app スタック(各 Lambda は SSM のタグを deploy 時に解決)
```

CDK の `BucketDeployment` や Docker アセットは使わない。成果物の配置は CDK の外で人間(または CI)が行う。

### Lambda のデプロイ単位

関数ごとに ECR リポジトリと「デプロイ中のイメージタグ」を持つ SSM パラメータを持ち、関数単位でデプロイのタイミングを分けられる。

```text
ECR  {stage}-snap-kakeibo-{function}:{git sha}
SSM  /{stage}/snap-kakeibo/functions/{function}/image-tag = {git sha}
```

`image:push` が push 後に SSM を更新し、App スタックは `AWS::SSM::Parameter::Value` 型の CloudFormation パラメータでタグを deploy 時に解決する。
CDK CLI は SSM 由来のパラメータがあるとテンプレートに差分が無くても deploy をスキップしないため、タグの更新だけで反映される。

---

## CloudFront

React のビルド成果物を S3 の frontend バケットに置き、CloudFront から OAC で配信する。

```text
ブラウザ
  |
  | HTTPS
  v
CloudFront
  |
  | OAC (SigV4)
  v
S3 frontend bucket (公開アクセスはブロック)
```

| 項目 | 値 | 理由 |
| --- | --- | --- |
| オリジンアクセス | OAC | バケットを公開しない。OAI は非推奨 |
| バケットポリシー | `cloudfront.amazonaws.com` に `s3:GetObject`、`AWS:SourceArn` を自アカウントの `distribution/*` で制限 | Distribution は app スタック側にあり、ARN を storage 側から参照すると循環するため |
| エラー応答 | 403 / 404 → `/index.html` (200) | SPA のルーティング。OAC 経由で存在しないキーは 403 になる |
| 価格クラス | PriceClass_200 | 日本を含む。PriceClass_100 は北米・欧州のみ |
| 独自ドメイン | 使わない | `*.cloudfront.net` で運用する |

キャッシュの無効化は `task front:push` が `/*` で行う。

### コスト

```text
CloudFront  月 1TB 転送、1,000 万リクエストまで無料
S3          数円
```

---

## S3

### バケット

| バケット | 用途 | スタック |
| --- | --- | --- |
| `{stage}-snap-kakeibo-receipt` | レシート画像(`receipts/`)と OpenAI の生レスポンス JSON(`analysis-results/`) | storage |
| `{stage}-snap-kakeibo-front` | React のビルド成果物。CloudFront から配信 | storage |

### Object Key

```text
receipts/{user_id}/{analysis_request_id}/original.jpg
analysis-results/{user_id}/{analysis_request_id}/{attempt}/{response_id}.json
```

例:

```text
receipts/01JUSERXXX/01JUPLOADXXX/original.jpg
analysis-results/01JUSERXXX/01JUPLOADXXX/1/resp_01JRESPONSEXXX.json
```

S3キーはクライアントから指定させず、Lambda側で生成する。`analysis-results/` は読み取り結果をあとから検証するための生レスポンスで、業務処理からは参照しない。

---

## S3イベント

画像アップロード完了後、S3 ObjectCreatedイベント(`receipts/` プレフィックス)をSQSへ送り、Analyze Lambdaを起動する。

```text
S3
  |
  | ObjectCreated (receipts/*)
  v
Analyze Queue (SQS)
  |
  v
Analyze Lambda
```

S3イベント通知はFIFOキューに送れないため、標準キューを使う。重複配信はLambda側の冪等性で吸収する。

---

## レシート解析(OpenAI API)

レシートの読み取りと明細カテゴリ分類は、OpenAI Responses API に画像を直接渡して1回の呼び出しで行う。OCR サービスは使わない。

Textract を使わない理由: Textract の対応言語は英語・フランス語・ドイツ語・イタリア語・ポルトガル語・スペイン語のみで日本語に対応しておらず、日本のレシートでは品目が読めない。

```text
S3
 |
 | ObjectCreated
 v
Analyze Queue (SQS)
 |
 v
Analyze Lambda
 |
 | 1. analysis_requests を ANALYZING に更新
 | 2. S3 から画像取得、長辺 2048px に縮小、JPEG で Base64
 | 3. Responses API
 |      input_image(Base64) + 指示文
 |      Structured Outputs(JSON Schema)
 |      → 店名 / 購入日 / 合計金額 / 明細(品目・金額・数量・カテゴリ)
 | 4. 生レスポンスを S3 analysis-results/ に保存
 | 5. 検証(合計・購入日・明細件数)
 v
DynamoDB TransactWriteItems
```

### 呼び出しパラメータ

| 項目 | 値 | 理由 |
| --- | --- | --- |
| モデル | 環境変数 `OPENAI_MODEL`(初期値 `gpt-5.6-terra`) | 非機密値としてCDKのstage設定で管理する |
| reasoning.effort | 環境変数 `OPENAI_REASONING_EFFORT`(初期値 `medium`) | 精度・レイテンシ・思考トークン数をログで実測して調整する |
| 画像 | 長辺 2048px を上限に縮小した JPEG(拡大はしない)を Base64 の data URL で渡す。上限は環境変数 `IMAGE_MAX_EDGE` で変更可 | パッチ方式のモデルは原寸でトークンを数えるため、縮小の効果が大きい。S3 の URL を渡すと Presigned URL の発行と公開範囲の管理が要る。細長いレシートは縮小で文字が潰れやすいので、実画像で読み取り精度を評価してから値を決める |
| input_image.detail | `high` を明示 | `auto` に任せない。tile 方式のモデルで `low` に落ちると品目が読めない |
| 出力 | `text.format = json_schema`、`strict = true`、`max_output_tokens = 4096` | 自由文を禁止する。それでも refusal / incomplete は返り得るので、判定は `docs/backend.md` の「レスポンスの判定」に従う |
| store | `false` を明示 | 省略時は OpenAI 側に 30 日保存される。レシートは個人情報なので保存させない。`previous_response_id` は使わないので困らない |
| 指示文 | 固定文を先頭に置く | cached input(通常の 1/10)を効かせる |

### データの保持

| 場所 | 内容 | 保持 |
| --- | --- | --- |
| OpenAI | リクエストとレスポンス | `store: false` で保存させない。不正利用監視のためのログは OpenAI 側のポリシーで最大 30 日保持される場合がある(ZDR は個人利用では申請しない) |
| S3 `receipts/` | 元画像 | 削除しない。請求詳細画面のプレビューで使う |
| S3 `analysis-results/` | OpenAI の生レスポンス JSON | ライフサイクルルールで 90 日後に削除。検証用途なので長期保持しない |
| CloudWatch Logs | Lambda のログ | 30 日。画像や生レスポンスの本文はログに出さない。正常時は`usage`と件数、失敗時はHTTP status、OpenAIのerror type/code/message、response_id、生レスポンスのS3キーを出す |

### コスト目安

モデル別の対応effort、公式トークン単価、レシート解析精度の期待値、評価方法は[レシート解析モデルの比較](../receipt-analysis-models.md)を正とする。

レシート1枚あたりの料金は画像サイズ、画像detail、指示文、出力・推論トークンによって変わるため、固定の円換算値は置かない。実レスポンスの`usage`から1枚あたりと成功1件あたりのUSDを計測する。

### タイムアウトとリトライ

| 項目 | 値 |
| --- | --- |
| Analyze Lambda タイムアウト | 3分 |
| OpenAI 呼び出しの合計上限 | 120秒。その中でエクスポネンシャルバックオフ付きリトライ |
| リトライ対象 | 429 / 500 / 502 / 503 / 504 / ネットワーク一時エラー |
| リトライしない | 400系の恒久エラー、JSON Schema 不一致 |

120秒以内に成功しなければエラーを返して SQS のリトライに任せる。`maxReceiveCount` 回失敗すると DLQ に入り、メールで通知される。恒久エラーは `FAILED`(`ANALYSIS_FAILED`)を記録して正常終了する。

### SSM パラメータ

```text
/{stage}/snap-kakeibo/openai/api-key           SecureString
```

Lambda はAPIキーを起動時にSSMから取得し、コンテナが生きている間はメモリに保持する。モデルとreasoning effortはLambda環境変数から取得する。

---

## SNS

DLQ アラームのメール通知にだけ使う。Textract 完了通知用のトピックは持たない。

```text
CloudWatch Alarm
  |
  v
SNS (alert トピック)
  |
  v
メール
```

メール購読の確認状態を失わないよう、トピックは storage スタックに置く。

---

## SQS

非同期処理のバッファとDLQとして使う。オーケストレーションには使わない。

### キュー

| キュー | 起動元 | 起動先 | DLQ |
| --- | --- | --- | --- |
| Analyze Queue | S3 ObjectCreated、再解析 API | Analyze Lambda | Analyze DLQ |

メッセージは2種類あり、Analyze Lambda はどちらも `user_id + analysis_request_id` に解決してから同じ処理を呼ぶ。

```text
S3 イベント通知     S3 のキーから user_id / analysis_request_id を取り出す。attempt = 1
再解析 API          {"user_id": "...", "analysis_request_id": "...", "attempt": 2, "trigger": "RETRY"}
```

`attempt` はメッセージ側で固定する。DynamoDB の現在値を使うと、遅れて届いた古いメッセージが新しい試行として処理される。

### SQSを挟む理由

```text
リトライ回数と間隔を maxReceiveCount と可視性タイムアウトで制御できる
  (Lambda非同期呼び出しは2回固定、超過後はイベント破棄)

失敗イベントがDLQに残り、redriveで再処理できる
  (追加コード不要、Lambdaが冪等なので戻すだけで安全)

Lambdaの同時実行数を絞ることでOpenAI APIのレート制限に当たりにくくする
  (一括アップロード時のスロットリング対策)

Lambda自体が停止していてもメッセージがキューに残る
```

API Gateway経由の同期APIには挟まない。ユーザーが結果を待つ処理なので、失敗はそのままエラーレスポンスで返す。

### 設定値

| 項目 | 値 | 理由 |
| --- | --- | --- |
| バッチサイズ | 1 | 1メッセージ = 1画像にし、部分失敗の扱いを不要にする |
| 可視性タイムアウト | Lambdaタイムアウトの6倍 | AWS推奨。Lambda 3分なら18分 |
| maxReceiveCount | 3 | 一時エラーの再試行として十分。それ以上は永久失敗の可能性が高い |
| DLQ保持期間 | 14日(最大) | 気づくまでの猶予を最大に取る |
| Analyze Lambda同時実行数 | 5 | OpenAI APIのレート制限を避ける |

### Lambdaの戻り値

SQS起動のLambdaでは「正常終了 = メッセージ削除」「エラー = キューに戻りリトライ」となる。

```text
正常終了(メッセージ削除)
  処理成功
  冪等スキップ
  永久失敗を analysis_requests に FAILED として記録済み

エラー(SQSリトライ)
  一時失敗(Throttling、TransactionConflict、ネットワークエラー)
  想定外の例外
```

永久失敗をエラーで返すと無駄なリトライの末にDLQへ行くだけになるため、`FAILED` を記録したら正常終了させる。

### 注意点

```text
SQS標準キューも at-least-once
  重複配信では OpenAI を二重に呼ぶことがある(1枚 ¥1 未満なので許容)
  DynamoDB 側は status / attempt 条件で二重登録を防ぐ

DLQで拾えるのはLambdaが失敗したケースのみ
  イベント自体が届かない、ユーザーがPUTしない、は拾えない
  → 画面側の停滞表示で気づき、手動で再実行する
```

---

## CloudWatch Alarm

各DLQの `ApproximateNumberOfMessagesVisible > 0` を監視し、通知用SNSトピック経由でメール通知する。

```text
Analyze DLQ --> CloudWatch Alarm --> SNS --> メール
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

```text
SecureString
  /{stage}/snap-kakeibo/auth/password-pepper
  /{stage}/snap-kakeibo/auth/jwt-secret
  /{stage}/snap-kakeibo/openai/api-key
```

ティアはStandardを使う(4KB以内、無料)。Secrets Managerは固定費がかかるため使わない。

CloudFormation は SecureString を作れないため、CDK ではなく `infra/cmd/ensure-parameters` が storage スタックの deploy 前に「無ければ作る」。Pepper と JWT 署名鍵は乱数で生成し、OpenAI の API キーは環境変数 `OPENAI_API_KEY` があればその値、無ければ `UNSET` で作る。既存の値は上書きしない。モデル名と reasoning effort は `infra/config` のstage設定からLambda環境変数へ渡す。

CDKはパラメータ名を環境変数としてLambdaへ渡し、Lambdaに `ssm:GetParameter` の権限を付ける。

---

## 採用しないもの

初期構成では以下は使用しない。

```text
Cognito
Step Functions
RDS
ECS
EventBridge Scheduler
Amazon Textract(日本語非対応)
```

理由:

```text
個人利用ではオーバースペック

Lambda + DynamoDB中心の構成で十分

固定費を極力持たない
```

SQSはオーケストレーションには使わないが、非同期処理のバッファとDLQとして使う。

停滞検知の定期実行(EventBridge Scheduler)は持たず、画面側の経過時間判定と手動再実行で対応する。
