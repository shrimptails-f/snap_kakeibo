# 命名変換表

## 目的

本書は、既存実装の名称とDDDで採用する業務上の名称の対応を定める。

ドメイン層では[ユビキタス言語](./ubiquitous-language.md)に合わせた名称を使う。既存API、DynamoDB、Lambdaなど外部契約の名称は、互換性を維持するため直ちには変更せず、境界で変換する。

## 移行区分

| 区分 | 意味 |
| --- | --- |
| 今変更する | 新しいドメインモデルや内部コードでは新名称を使用する |
| 接続時に変更する | 既存featureを新しいモデルへ接続するときに内部名称を変更する |
| 当面維持する | API、DynamoDB、Lambdaなど互換性へ影響するため、既存名称を境界で維持する |
| 将来検討する | APIバージョン変更やデータ移行を伴うため、独立した変更として判断する |

## コンテキストとpackage

| 対象 | 現在・移行途中の名称 | 推奨名称 | 日本語 | 区分 | 理由・補足 |
| --- | --- | --- | --- | --- | --- |
| 家計簿コンテキスト | `internal/billing` | `internal/ledger` | 家計簿 | 接続時に変更する | 支出だけでなく月次集計も含むため、コンテキスト全体の名前は`expense`より`ledger`が広さに合う |
| 家計簿コンテキスト（PR #55） | `internal/expense` | `internal/ledger` | 家計簿 | 接続前に変更可能 | 現在は未接続なので、既存APIへの影響なしに変更できる |
| 画像受付・解析 | `internal/upload`、`internal/analysis` | 現状維持 | 画像受付・解析 | 当面維持する | 受付と解析で責務が分かれており、既存feature構成と一致する |
| 認証 | `internal/auth` | 現状維持 | 認証 | 当面維持する | 業務上の意味とコード名が一致している |
| 共有ドメイン | `internal/common/domain` | 現状維持 | 共有ドメイン | 当面維持する | 複数コンテキストで同じ意味と制約を持つ型だけを置く |

`ledger`は帳簿一般を表す。日本語での会話では「家計簿コンテキスト」と呼び、コード上のpackage名として`ledger`を使う。

## 集約・エンティティ・読み取りモデル

| 現在の名称 | 推奨名称 | 日本語 | 区分 | 備考 |
| --- | --- | --- | --- | --- |
| `Billing` | `Expense` | 支出 | 今変更する | 購入、取引、請求、返金など、家計へ金額上の影響を与えるもの |
| `BillingDetail` | `ExpenseDetail` | 支出明細 | 今変更する | 支出に含まれる商品またはサービスの1行 |
| `UploadHistory` | `AnalysisRequest` | 解析依頼 | 接続時に変更する | 履歴レコードではなく、画像受付から解析終端までを管理する集約として扱う |
| `Receipt`（OpenAI出力と検証対象を兼用） | `AnalysisResult` | 解析結果 | 接続時に変更する | OpenAIレスポンスそのものとは分離する |
| `Detail`（解析結果内） | `AnalyzedDetail` | 解析明細 | 接続時に変更する | 支出として確定する前の解析結果であることを示す |
| `MonthlySummary` | `MonthlySummary` | 月次集計 | 維持する | 家計簿コンテキスト内の読み取りモデル |
| `User` | `User` | 利用者 | 維持する | 現在の意味と一致している |

## ID

| 現在の名称 | 推奨名称 | 日本語 | 区分 | 境界での扱い |
| --- | --- | --- | --- | --- |
| `BillingID` | `ExpenseID` | 支出ID | 今変更する | APIの`billing_id`、DBの`billing_id`とはrepository・handlerで変換する |
| `BillingDetailID` / `DetailID` | `ExpenseDetailID` | 支出明細ID | 今変更する | DBの`detail_id`は当面維持する |
| `UploadID` | `AnalysisRequestID` | 解析依頼ID | 接続時に変更する | APIの`upload_id`、S3キー、DB属性は当面維持する |
| `UserID` | `UserID` | 利用者ID | 維持する | 全コンテキストで同じ意味を持つ共有ID |

## 金額

| 現在の名称 | 推奨名称 | 日本語 | 計算・意味 | 区分 |
| --- | --- | --- | --- | --- |
| `OriginalAmount` / `original_amount` | `ReadAmount` / `read_amount` | 読取金額 | 店舗側の値引きや税を反映済みの、レシートに記載された最終支払合計 | Goは今変更、DBは当面維持 |
| `DiscountAmount` / `discount_amount` | `AdjustmentAmount` / `adjustment_amount` | 調整額 | 利用者が加減する符号付き金額。減額は負数、増額は正数 | Goは今変更、DBは移行が必要 |
| `FinalAmount` / `final_amount` | `RecordedAmount` / `recorded_amount` | 計上額 | `読取金額 + 調整額`。月次集計へ反映し、負数を許容する | Goは今変更、DBは移行が必要 |
| 解析結果の`TotalAmount` / `total_amount` | `ReadAmount` / `read_amount` | 読取金額 | OpenAIから得た最終支払合計 | ドメイン変換時に変更する |
| 明細の`Amount` / `amount` | `DetailAmount` / `amount` | 明細金額 | 数量を反映した支出明細1行の金額 | Goでは型名を明確化、外部属性は維持 |
| 月次集計の`TotalAmount` / `total_amount` | `TotalRecordedAmount` / `total_recorded_amount` | 計上額合計 | 対象月の`RecordedAmount`の合計 | Goは接続時に変更、DBは当面維持 |
| 月次集計の`BillingCount` / `billing_count` | `ExpenseCount` / `expense_count` | 支出件数 | 対象月に含まれる支出の件数 | Goは接続時に変更、DBは当面維持 |
| 月次集計の`DetailCount` / `detail_count` | `DetailCount` / `detail_count` | 明細件数 | 対象月に含まれる支出明細の件数 | 維持する |

既存DBの`discount_amount`は「正数を差し引く」意味で、新しい`AdjustmentAmount`は「符号付きで加算する」意味である。名称だけを置換してはならず、移行時には符号の反転が必要になる。

```text
旧: final_amount = original_amount - discount_amount
新: recorded_amount = read_amount + adjustment_amount

移行時: adjustment_amount = -discount_amount
```

## 日付・月

| 現在の名称 | 推奨名称 | 日本語 | 区分 | 備考 |
| --- | --- | --- | --- | --- |
| `PurchasedAt` | `PurchaseDate` | 購入日 | 今変更する | 時刻を持たないため、Goの型名では`At`より`Date`が正確 |
| `purchased_at` | `purchased_at` | 購入日 | 当面維持する | API・DB互換性のため維持し、境界で`PurchaseDate`へ変換する |
| `YearMonth` / `year_month` | `YearMonth` / `year_month` | 対象月 | 維持する | `YYYY-MM`を表し、現在の意味と一致している |
| `IsEdited` / `is_edited` | `Edited` / `is_edited` | 編集済み | Goは変更、DBは維持 | Goでは真偽値のgetterを`Edited()`として扱い、保存属性は互換性を維持する |
| `CreatedAt` / `UpdatedAt` | 現状維持 | 作成日時・更新日時 | 維持する | 実際に時刻を持つため`At`が適切 |

## 解析と再実行

| 現在の名称 | 推奨名称 | 日本語 | 区分 | 備考 |
| --- | --- | --- | --- | --- |
| `RetryUpload` | `RetryAnalysis` | 再解析 | 接続時に変更する | 画像の再アップロードではなく、新しい解析試行を開始する操作 |
| `retry-upload` Lambda | 当面維持 | 再解析Lambda | 当面維持する | Lambda名変更はinfraとデプロイに影響する |
| `/uploads/{upload_id}/retry` | 当面維持 | 再解析API | 当面維持する | 外部API互換性を優先する |
| `attempt` | `Attempt` | 解析試行番号 | 維持する | 同一試行のSQS再配信では増やさず、再解析時だけ増やす |
| `Retry`（SQS・HTTP・OpenAIで曖昧） | `RetryAnalysis`、`Redelivery`、`TransientRetry` | 再解析・再配信・一時エラー再試行 | 接続時に区別する | 同じ「retry」で異なる操作を表さない |
| `Job` | `AnalysisJob` | 解析ジョブ | 接続時に変更する | package外や複数ジョブが並ぶ箇所では対象を明確にする |
| `Failure` | `FailureReason` / `AnalysisFailure` | 失敗理由・解析失敗 | 接続時に変更する | 状態に保存する値と処理結果のエラーを区別する |

## ユースケース・repository

| 現在の名称 | 推奨名称 | 区分 | 備考 |
| --- | --- | --- | --- |
| `GetBilling` | `GetExpense` | 接続時に変更する | HTTPパスは既存の`/billings/{billing_id}`を維持してよい |
| `GetBillingUsecaseInterface` | `GetExpenseUsecaseInterface` | 接続時に変更する | handler境界で旧API名と対応付ける |
| `BillingRepository` | `ExpenseRepository` | 接続時に変更する | domain/application側の能力名を業務用語へ合わせる |
| `BillingDetailRepository` | `ExpenseDetailRepository` | 接続時に変更する | 支出集約単位で保存する場合は1つの`ExpenseRepository`への統合も検討する |
| `BillingRegistrar` | `ExpenseRegistrar` | 接続時に変更する | 解析結果から支出を登録する能力 |
| `ListExpenses` | 現状維持 | 維持する | すでにユビキタス言語と一致している |
| `RecalculateMonthlySummary` | `RebuildMonthlySummary` | Go内部は変更する | 差分計算ではなく正本から作り直すため`Rebuild`が正確 |
| `/monthly-summaries/{yyyy-MM}/recalculate` | 当面維持 | 当面維持する | 外部APIの破壊的変更を避ける |

## API・Lambda・DynamoDB

次の名称はドメイン用語とは異なるが、外部契約または保存済みデータとの互換性を優先して当面維持する。

| 種別 | 現在の名称 | ドメインでの対応 | 方針 |
| --- | --- | --- | --- |
| API | `GET /billings/{billing_id}` | 支出を取得する | handlerで`billing_id`を`ExpenseID`へ変換する |
| API | `PATCH /billings/{billing_id}` | 支出を編集する | handlerで支出編集ユースケースへ変換する |
| API | `POST /uploads/{upload_id}/retry` | 解析依頼を再解析する | handlerで`AnalysisRequestID`へ変換する |
| Lambda | `get-billing` | 支出取得 | デプロイ名は維持し、内部の型・usecase名だけ変更できる |
| Lambda | `retry-upload` | 再解析 | デプロイ名は維持し、内部の型・usecase名だけ変更できる |
| DynamoDB | `billings` | 支出 | repositoryの永続化モデルで対応付ける |
| DynamoDB | `billing_details` | 支出明細 | repositoryの永続化モデルで対応付ける |
| DynamoDB | `upload_histories` | 解析依頼 | repositoryの永続化モデルで対応付ける |
| DynamoDB | `monthly_summaries` | 月次集計 | 名称を維持する |
| DynamoDB属性 | `billing_id` | `ExpenseID` | 読み書き時に変換する |
| DynamoDB属性 | `upload_id` | `AnalysisRequestID` | 読み書き時に変換する |
| DynamoDB属性 | `original_amount` | `ReadAmount` | 読み書き時に変換する |
| DynamoDB属性 | `discount_amount` | `AdjustmentAmount` | 現行値の符号を反転して変換する |
| DynamoDB属性 | `final_amount` | `RecordedAmount` | 読み書き時に変換する |

テーブル名や属性名を将来変更する場合は、二重書き、バックフィル、読み取りの切り替え、旧属性の廃止というデータ移行が必要になる。DDD導入だけを理由に即時変更しない。

## 状態とカテゴリ

| 現在の名称 | 推奨名称 | 区分 | 備考 |
| --- | --- | --- | --- |
| `UPLOADING` | 現状維持 | 維持する | アップロード待ち |
| `ANALYZING` | 現状維持 | 維持する | 解析中 |
| `SUCCEEDED` | 現状維持 | 維持する | 登録完了 |
| `NO_DATA` | 現状維持 | 維持する | 明細0件のため登録対象なし |
| `FAILED` | 現状維持 | 維持する | 解析失敗 |
| `CategorySourceAI` / `AI` | 現状維持 | 維持する | AIがカテゴリを決定した |
| `CategorySourceUser` / `USER` | 現状維持 | 維持する | 利用者がカテゴリを決定した |
| 既存カテゴリ一覧 | `social`を追加 | 接続時に変更する | OpenAI Schema、集計属性、API、画面を同時に更新する |

## 移行順序

1. 新規ドメインコードで`AnalysisRequest`、`Expense`、`ExpenseDetail`と新しい金額名を使用する。
2. 家計簿コンテキストのpackage名を`ledger`に確定し、未接続の`internal/expense`を必要なら移動する。
3. infrastructureに既存DynamoDBレコードとドメインモデルの変換を実装する。
4. applicationのusecase・interfaceを新しい名称へ変更する。
5. handlerで既存APIの`billing_id`、`upload_id`を新しいID型へ変換する。
6. `social`をOpenAI Schema、集計、API、画面へ同時に追加する。
7. APIパスやDynamoDB名の変更は、必要性が生じた場合だけ別Issueで実施する。

## 命名原則

- ドメイン層ではユビキタス言語を優先する。
- APIやDynamoDBの既存名を、そのままドメインモデル名にしない。
- 同じ言葉で異なる処理を表さない。特に「retry」と「amount」は具体的な意味を付ける。
- `At`は時刻、`Date`は暦日、`YearMonth`は暦月に使用する。
- 永続化名を変更するときは、コードのrenameではなくデータ移行として計画する。
