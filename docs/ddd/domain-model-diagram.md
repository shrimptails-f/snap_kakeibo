# ドメインモデル図

## 目的

本書は、snap_kakeibo のドメインモデル、集約境界、モデル間の関係を図示する。

用語の意味は[ユビキタス言語](./ubiquitous-language.md)、境界とルールの詳細は[ドメイン境界とモデル](./domain-model.md)を正とする。本図はドメインモデルを表し、DynamoDBの属性やAPIのキーは[DB設計](../infra/database.md)と各画面の仕様を参照する。

## コンテキスト間の関係

```mermaid
flowchart LR
    Auth[認証コンテキスト]
    Analysis[画像受付・解析コンテキスト]
    Household[家計簿コンテキスト]
    Summary[月次集計<br/>読み取りモデル]

    Auth -- 認証済み利用者ID --> Analysis
    Auth -- 認証済み利用者ID --> Household
    Analysis -- 検証済み解析結果 --> Household
    Household -- 支出から投影・再構築 --> Summary
```

月次集計は独立したコンテキストではなく、家計簿コンテキスト内の読み取りモデルである。

## 画像受付・解析コンテキスト

```mermaid
classDiagram
    direction LR

    class AnalysisRequest {
        <<集約ルート>>
        +AnalysisRequestID id
        +UserID userID
        +ReceiptImage image
        +AnalysisStatus status
        +Attempt currentAttempt
        +DateTime uploadExpiresAt
        +ExpenseID expenseID
        +FailureReason failureReason
        +DateTime createdAt
        +DateTime updatedAt
        +startAnalysis(attempt, now)
        +complete(attempt, expenseID, now)
        +markNoData(attempt, now)
        +fail(attempt, reason, now)
        +retry(now)
    }

    class ReceiptImage {
        <<値オブジェクト>>
        +ObjectReference reference
        +FileName fileName
        +ContentType contentType
    }

    class Attempt {
        <<値オブジェクト>>
        +int value
        +next() Attempt
        +matches(other) bool
    }

    class AnalysisStatus {
        <<列挙>>
        UPLOADING
        ANALYZING
        SUCCEEDED
        NO_DATA
        FAILED
    }

    class FailureReason {
        <<値オブジェクト>>
        +FailureCode code
        +string safeMessage
    }

    class ReceiptReading {
        <<読み取り内容>>
        +string storeName
        +string purchaseDate
        +int64 readAmount
        +List~ReadDetail~ details
    }

    class AnalysisResult {
        <<解析結果>>
        +StoreName storeName
        +PurchaseDate purchaseDate
        +ReadAmount readAmount
        +List~AnalyzedDetail~ details
        +hasData() bool
    }

    class AnalyzedDetail {
        <<値オブジェクト>>
        +string name
        +DetailAmount amount
        +Quantity quantity
        +Category category
    }

    AnalysisRequest *-- "1" ReceiptImage : 解析対象
    AnalysisRequest *-- "1" Attempt : 現在の試行
    AnalysisRequest --> "1" AnalysisStatus : 状態
    AnalysisRequest o-- "0..1" FailureReason : 失敗時のみ
    ReceiptReading ..> AnalysisResult : validateReading(now)
    ReceiptReading ..> FailureReason : 検証違反
    AnalysisResult *-- "0..50" AnalyzedDetail : 検証後
```

読み取り内容(`ReceiptReading`)はOpenAIのレスポンスを解釈した検証前の値で、読めなかった項目を持たないことがある。`validateReading`が業務上の検証を行い、検証済みの解析結果(`AnalysisResult`)か失敗理由(`FailureReason`)を返す。明細が0件の場合は支出を作らず、解析依頼を`NO_DATA`にする。

解析中の依頼を再解析可能とする条件と、停滞と判定する時間は未決定のため、図では`retry`の詳細な事前条件を固定しない。

## 家計簿コンテキスト

```mermaid
classDiagram
    direction LR

    class Expense {
        <<集約ルート>>
        +ExpenseID id
        +UserID userID
        +AnalysisRequestID sourceRequestID
        +StoreName storeName
        +PurchaseDate purchaseDate
        +ReadAmount readAmount
        +AdjustmentAmount adjustmentAmount
        +RecordedAmount recordedAmount
        +bool edited
        +changeStoreName(name)
        +changePurchaseDate(date)
        +adjustAmount(amount)
        +renameDetail(detailID, name)
        +changeDetailAmount(detailID, amount, quantity)
        +changeDetailCategory(detailID, category)
    }

    class ExpenseDetail {
        <<エンティティ>>
        +ExpenseDetailID id
        +string name
        +DetailAmount amount
        +Quantity quantity
        +Category category
        +CategorySource categorySource
    }

    class PurchaseDate {
        <<値オブジェクト>>
        +Date value
        +yearMonth() YearMonth
    }

    class ReadAmount {
        <<値オブジェクト>>
        +int64 yen
    }

    class AdjustmentAmount {
        <<値オブジェクト>>
        +int64 yen
    }

    class RecordedAmount {
        <<値オブジェクト>>
        +int64 yen
        +from(readAmount, adjustmentAmount) RecordedAmount
    }

    class Category {
        <<列挙>>
        food
        daily_goods
        medical
        transport
        utilities
        entertainment
        social
        clothing
        education
        other
        unknown
    }

    class CategorySource {
        <<列挙>>
        AI
        USER
    }

    class MonthlySummary {
        <<読み取りモデル>>
        +UserID userID
        +YearMonth yearMonth
        +RecordedAmount totalRecordedAmount
        +int expenseCount
        +int detailCount
        +CategoryTotals categoryTotals
        +int64 version
    }

    Expense *-- "1..50" ExpenseDetail : 支出明細
    Expense *-- "1" PurchaseDate
    Expense *-- "1" ReadAmount
    Expense *-- "1" AdjustmentAmount
    Expense *-- "1" RecordedAmount : 導出値
    ExpenseDetail --> "1" Category
    ExpenseDetail --> "1" CategorySource
    MonthlySummary ..> Expense : 支出から投影・再構築
    MonthlySummary ..> ExpenseDetail : カテゴリ別金額を集計
```

## コンテキスト境界での変換

```mermaid
flowchart LR
    External[OpenAIレスポンス<br/>外部モデル]
    Reading[読み取り内容<br/>画像受付・解析]
    Result[解析結果<br/>画像受付・解析]
    Expense[支出<br/>家計簿の集約ルート]
    Summary[月次集計<br/>読み取りモデル]

    External -- 解釈 --> Reading
    Reading -- 検証 --> Result
    Result -- 変換 --> Expense
    Expense -- 投影・再構築 --> Summary
```

- OpenAIレスポンスの型を解析結果として使用しない。
- 解析結果の型を支出集約へ直接持ち込まず、application層で変換する。
- 月次集計は支出と支出明細から導出し、支出へ逆反映しない。

## 集約境界のルール

- 解析依頼と支出は別の集約とし、互いをIDで参照する。
- 支出明細は支出集約の内部に置き、支出を経由して変更する。
- 1つの支出が持つ支出明細は1件以上50件以下とする。
- 計上額は`読取金額 + 調整額`から導出し、直接変更しない。
- 計上額は返金を表現できるように負数を許容する。
- 月次集計は集約ではなく、家計簿コンテキストの読み取りモデルとする。
- DynamoDBの条件式とトランザクションは、集約のルールを並行処理下でも保証するために使用する。
