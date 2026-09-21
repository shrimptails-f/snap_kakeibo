# 命名変換表

## 目的

本書は、旧実装の名称と、DDDで採用した業務上の名称の対応を定める。

コード、API、DynamoDB、Lambda、設定、画面、文書はすべて[ユビキタス言語](./ubiquitous-language.md)に合わせた名称へ全面移行済みである。旧名称は互換性のために維持せず、境界で変換もしない。本書は「何が何に変わったか」を示す記録であり、旧名称を新たに使う根拠にはしない。

## 移行方針

- 後方互換性は維持しない。旧API、旧DynamoDBスキーマ、旧Lambda名、旧データの移行経路は用意しない。
- 旧テーブルのバックフィル、二重書き、dual read、deprecated alias、互換用type aliasは行わない。
- 開発環境の既存データは破棄し、新スキーマで再作成する。CDK上では旧テーブル・旧ECR・旧ロググループを置換する。
- backend、infra、画面、テスト、文書を一度の変更で整合させる。

## コンテキストとpackage

| 旧名称 | 新名称 | 日本語 | 備考 |
| --- | --- | --- | --- |
| `internal/billing`、`internal/expense` | `internal/ledger` | 家計簿 | 支出集約と月次集計(読み取りモデル)を持つ。package名は帳簿一般を表す`ledger` |
| `internal/upload` | `internal/upload` | 画像受付 | 解析依頼の登録、一覧、再解析。解析依頼の集約は`analysis/domain`を共有する |
| `internal/analysis` | `internal/analysis` | 解析 | 解析依頼の集約、解析ジョブ、読み取り内容の検証、解析結果 |
| `internal/auth` | `internal/auth` | 認証 | 変更なし |
| `internal/common/domain` | `internal/common/domain` | 共有ドメイン | 複数コンテキストで同じ意味を持つID、購入日、対象月、金額、カテゴリ |

`ledger`は帳簿一般を表す。日本語での会話では「家計簿コンテキスト」と呼ぶ。

## 集約・エンティティ・読み取りモデル

| 旧名称 | 新名称 | 日本語 | 備考 |
| --- | --- | --- | --- |
| `Billing`(`billing/domain`、`analysis/domain`) | `ledger/domain.Expense` | 支出 | 解析からの登録と参照の両方で同じ集約を使う |
| `BillingDetail` | `ledger/domain.ExpenseDetail` | 支出明細 | 支出集約の内部エンティティ |
| `UploadHistory`(`upload/domain`) | `analysis/domain.AnalysisRequest` | 解析依頼 | `upload/domain`は型エイリアスで参照する。永続化からの復元で状態と付随する値の整合を検証する |
| `Receipt`(OpenAI出力と検証対象を兼用) | `analysis/domain.ReceiptReading`(検証前)と`AnalysisResult`(検証済み) | 読み取り内容・解析結果 | OpenAIのJSON形式は`infrastructure`の`receiptOutput`に閉じ込める |
| `Detail`(解析結果内) | `ReadDetail`(検証前)と`AnalyzedDetail`(検証済み) | 解析明細 | |
| `Job` | `AnalysisJob` | 解析ジョブ | キューのメッセージから復元する解析試行1件 |
| `Status`(analysis) | `AnalysisOutcome` | 解析試行の結末 | 状態に保存しない`SKIPPED`を含むため`AnalysisStatus`と分ける |
| `Status`(upload) | `AnalysisStatus` | 解析依頼の状態 | `UPLOADING` / `ANALYZING` / `SUCCEEDED` / `NO_DATA` / `FAILED` |
| `Failure` | `FailureReason` | 失敗理由 | 状態に保存する値。`AnalysisFailed` / `InternalFailure`で生成する |
| `RetryJob` | `RetryAnalysisJob` | 再解析ジョブ | |
| `MonthlySummary` | `MonthlySummary` | 月次集計 | 家計簿コンテキスト内の読み取りモデル |
| `User` | `User` | 利用者 | 変更なし |

## ID

| 旧名称 | 新名称 | 日本語 | API / DB |
| --- | --- | --- | --- |
| `BillingID` / `billing_id` | `ExpenseID` / `expense_id` | 支出ID | パス`{expenseId}`、属性`expense_id` |
| `BillingDetailID` / `DetailID` | `ExpenseDetailID` / `detail_id` | 支出明細ID | 属性`detail_id`は維持 |
| `UploadID` / `upload_id` | `AnalysisRequestID` / `analysis_request_id` | 解析依頼ID | パス`{analysisRequestId}`、属性・S3キー・SQSメッセージも`analysis_request_id` |
| `UserID` / `user_id` | `UserID` / `user_id` | 利用者ID | 変更なし |

## 金額

| 旧名称 | 新名称 | 日本語 | 計算・意味 |
| --- | --- | --- | --- |
| `OriginalAmount` / `original_amount` | `ReadAmount` / `read_amount` | 読取金額 | 店舗側の値引きや税を反映済みの、レシートに記載された最終支払合計 |
| `DiscountAmount` / `discount_amount` | `AdjustmentAmount` / `adjustment_amount` | 調整額 | 利用者が加減する符号付き金額。減額は負数、増額は正数 |
| `FinalAmount` / `final_amount` | `RecordedAmount` / `recorded_amount` | 計上額 | `read_amount + adjustment_amount`。月次集計へ反映し、負数を許容する |
| 解析結果の`TotalAmount` | `ReadAmount` | 読取金額 | OpenAIの出力(`total_amount`)を読み取り内容の`ReadAmount`として扱う |
| 明細の`Amount` / `amount` | `DetailAmount` / `amount` | 明細金額 | 数量を反映した支出明細1行の金額 |
| 月次集計の`TotalAmount` / `total_amount` | `TotalRecordedAmount` / `total_recorded_amount` | 計上額合計 | 対象月の`recorded_amount`の合計 |
| 月次集計の`BillingCount` / `billing_count` | `ExpenseCount` / `expense_count` | 支出件数 | |
| 月次集計の`DetailCount` / `detail_count` | `DetailCount` / `detail_count` | 明細件数 | 変更なし |

旧`discount_amount`は「正数を差し引く」意味で、新`adjustment_amount`は「符号付きで加算する」意味である。旧データは移行しないため符号の変換処理は持たない。

```text
旧: final_amount    = original_amount - discount_amount
新: recorded_amount = read_amount + adjustment_amount
```

## 日付・月

| 旧名称 | 新名称 | 日本語 | 備考 |
| --- | --- | --- | --- |
| `PurchasedAt` / `purchased_at` | `PurchaseDate` / `purchase_date` | 購入日 | 時刻を持たない暦日。OpenAIのJSON Schemaも`purchase_date` |
| `ExpiresAt` / `expires_at`(解析依頼) | `UploadExpiresAt` / `upload_expires_at` | アップロード期限 | 署名付きURLの期限 |
| `YearMonth` / `year_month` | `YearMonth` / `year_month` | 対象月 | 変更なし |
| `IsEdited` / `is_edited` | `Edited()` / `is_edited` | 編集済み | Goのgetterは`Edited()`、保存属性とAPIは`is_edited` |
| `CreatedAt` / `UpdatedAt` | 変更なし | 作成日時・更新日時 | 実際に時刻を持つため`At` |

## 解析と再実行

| 旧名称 | 新名称 | 日本語 | 備考 |
| --- | --- | --- | --- |
| `RetryUpload` | `RetryAnalysis` | 再解析 | 新しい解析試行を開始し、`attempt`を1増やす |
| `retry-upload` Lambda | `retry-analysis` | 再解析Lambda | |
| `POST /uploads/{upload_id}/retry` | `POST /analysis-requests/{analysis_request_id}/retry` | 再解析API | |
| `ListUploads` / `list-uploads` / `GET /months/{yyyy-MM}/uploads` | `ListAnalysisRequests` / `list-analysis-requests` / `GET /months/{yyyy-MM}/analysis-requests` | 解析依頼一覧 | |
| `attempt` | `Attempt` | 解析試行番号 | 同一試行のSQS再配信では増やさず、再解析時だけ増やす |
| SQSの`retry` | 同一試行の再配信 | 再配信 | `attempt`を進めない |
| OpenAIの`retry` | 一時エラーの再試行 | 一時エラー再試行 | 同じ呼び出しの中でバックオフ付きで再送する |

## ユースケース・repository

| 旧名称 | 新名称 | 備考 |
| --- | --- | --- |
| `GetBilling` / `GetBillingUsecaseInterface` | `GetExpense` / `GetExpenseUsecaseInterface` | 支出集約を支出明細ごと返す |
| `BillingFinder` + `BillingDetailLister` | `ExpenseFinder` | 支出集約単位で読むため1つに統合 |
| `BillingRegistrar` / `ErrBillingRejected` | `ExpenseRegistrar` / `ErrExpenseRejected` | 解析結果から支出を登録する |
| `UploadHistoryRepository`(analysis) | `AnalysisRequestRepository` | `MarkAnalyzing` / `MarkFailed` / `MarkNoData` |
| `UploadHistoryRepository` / `UploadRetryMarker` / `UploadHistoryLister`(upload) | `AnalysisRequestRepository` / `RetryAnalysisMarker` / `AnalysisRequestLister` | |
| `AnalyzeJobEnqueuer.EnqueueRetry` | `RetryAnalysisEnqueuer.EnqueueRetryAnalysis` | |
| `ErrUploadAlreadyExists` / `ErrUploadNotRetryable` | `ErrAnalysisRequestAlreadyExists` / `ErrAnalysisRequestNotRetryable` | |
| `AnalysisResult`(application、OpenAI応答) | `AnalyzerResponse` | domainの`AnalysisResult`(検証済み解析結果)と区別する |
| `RecalculateMonthlySummary` / `/recalculate` | `RebuildMonthlySummary` / `/rebuild` | 差分計算ではなく正本から作り直す |
| `ListExpenses` | 変更なし | |

## API・Lambda・DynamoDB

| 種別 | 旧名称 | 新名称 |
| --- | --- | --- |
| API | `GET /billings/{billing_id}` | `GET /expenses/{expense_id}` |
| API | `PATCH /billings/{billing_id}` | `PATCH /expenses/{expense_id}` |
| API | `POST /uploads/{upload_id}/retry` | `POST /analysis-requests/{analysis_request_id}/retry` |
| API | `GET /months/{yyyy-MM}/uploads` | `GET /months/{yyyy-MM}/analysis-requests` |
| API | `POST /monthly-summaries/{yyyy-MM}/recalculate` | `POST /monthly-summaries/{yyyy-MM}/rebuild` |
| API レスポンス | `upload_id`、`billing`、`billing_id`、`purchased_at`、`original_amount` / `discount_amount` / `final_amount` | `analysis_request_id`、`expense`、`expense_id`、`purchase_date`、`read_amount` / `adjustment_amount` / `recorded_amount` |
| Lambda | `get-billing` | `get-expense` |
| Lambda | `retry-upload` | `retry-analysis` |
| Lambda | `list-uploads` | `list-analysis-requests` |
| 環境変数 | `BILLINGS_TABLE` / `BILLING_DETAILS_TABLE` / `UPLOAD_HISTORIES_TABLE` | `EXPENSES_TABLE` / `EXPENSE_DETAILS_TABLE` / `ANALYSIS_REQUESTS_TABLE` |
| DynamoDB テーブル | `billings` / `billing-details` / `upload-histories` | `expenses` / `expense-details` / `analysis-requests` |
| DynamoDB SK | `BILLING#{billing_id}` / `UPLOAD#{upload_id}` | `EXPENSE#{expense_id}` / `ANALYSIS_REQUEST#{analysis_request_id}` |
| DynamoDB PK(明細) | `USER#{user_id}#BILLING#{billing_id}` | `USER#{user_id}#EXPENSE#{expense_id}` |
| DynamoDB GSI | `upload_month_index`、`UPLOAD_CREATED_AT#...` | `analysis_request_month_index`、`ANALYSIS_REQUEST_CREATED_AT#...` |
| DynamoDB type | `BILLING` / `BILLING_DETAIL` / `UPLOAD_HISTORY` | `EXPENSE` / `EXPENSE_DETAIL` / `ANALYSIS_REQUEST` |
| DynamoDB 属性 | `billing_id`、`upload_id`、`purchased_at`、`original_amount`、`discount_amount`、`final_amount`、`expires_at`、`total_amount`、`billing_count` | `expense_id`、`analysis_request_id`、`purchase_date`、`read_amount`、`adjustment_amount`、`recorded_amount`、`upload_expires_at`、`total_recorded_amount`、`expense_count` |
| ログ field | `upload_id` / `billing_id` | `analysis_request_id` / `expense_id` |
| SQS メッセージ | `{"user_id","upload_id","attempt","trigger"}` | `{"user_id","analysis_request_id","attempt","trigger"}` |

S3のオブジェクトキーは`receipts/{user_id}/{analysis_request_id}/original.jpg`と`analysis-results/{user_id}/{analysis_request_id}/{attempt}/{response_id}.json`で、形式は変えず名前の意味だけを改める。

## 変更しない外部名称

| 名称 | 理由 |
| --- | --- |
| OpenAI JSON Schemaの`total_amount` | レシートの「合計金額」そのものを表す外部モデルの項目。ドメインへ変換する時点で読取金額(`ReadAmount`)になる |
| 失敗コード`NO_TOTAL_AMOUNT` | 「合計金額を取得できなかった」というOpenAI出力に対する失敗の意味を保つ |
| `detail_id` / `detail_count` / `category_total_{category}` / `monthly_summaries` | ユビキタス言語と一致している |
| DynamoDB SDKの`BillingMode` | AWSの課金モードを表す語で、業務上の「請求」ではない |

## 状態とカテゴリ

| 名称 | 備考 |
| --- | --- |
| `UPLOADING` / `ANALYZING` / `SUCCEEDED` / `NO_DATA` / `FAILED` | 変更なし |
| `CategorySourceAI` / `AI`、`CategorySourceUser` / `USER` | 変更なし |
| カテゴリ | `social`(交際・会食)を追加。語彙は`internal/common/domain.Categories()`が正で、OpenAI Schema、検証、月次集計、APIレスポンス、画面の表示名がこれに従う |

## 命名原則

- ドメイン層ではユビキタス言語を優先し、API・DynamoDB・Lambdaもドメインと同じ語を使う。
- 同じ言葉で異なる処理を表さない。特に「retry」と「amount」は具体的な意味を付ける。
- `At`は時刻、`Date`は暦日、`YearMonth`は暦月に使用する。
- 永続化名を変更するときは、開発環境では作り直し、本番運用後はデータ移行として計画する。
