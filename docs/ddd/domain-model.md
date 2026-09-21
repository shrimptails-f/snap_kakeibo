# ドメイン境界とモデル

## 目的

本書は、snap_kakeibo の業務概念をどの境界で扱い、どのモデルが業務ルールを保証するかを定める。

用語の意味は[ユビキタス言語](./ubiquitous-language.md)、旧実装からの名称対応は[命名変換表](./naming-mapping.md)を正とする。

コード上の置き場所は、画像受付・解析コンテキストが`internal/upload`と`internal/analysis`、家計簿コンテキストが`internal/ledger`、認証コンテキストが`internal/auth`、共有ドメインが`internal/common/domain`である。

## 境界づけられたコンテキスト

### 画像受付・解析コンテキスト(`internal/upload`、`internal/analysis`)

レシート画像を受け付け、解析を実施し、検証済みの解析結果を家計簿コンテキストへ渡す。解析依頼の集約は`analysis/domain`に置き、`upload`はそれを共有する。

主な概念:

- レシート画像
- 解析依頼
- 解析試行
- 解析結果
- 解析失敗理由

このコンテキストは、支出の編集や月次集計を担当しない。

### 家計簿コンテキスト(`internal/ledger`)

利用者の支出と支出明細を管理し、計上額などの不変条件を保証する。月次集計もこのコンテキストの読み取りモデルとして扱う。

主な概念:

- 支出
- 支出明細
- 読取金額
- 調整額
- 計上額
- 購入日
- カテゴリ
- 対象月
- 月次集計

画像解析は支出を作る入力の一つであり、家計簿コンテキストはOpenAIや画像形式を認識しない。

### 認証コンテキスト

利用者の認証、アクセストークン、リフレッシュトークンを管理する。家計簿の中心ドメインとは分離し、他のコンテキストには認証済みの利用者IDだけを渡す。

## コンテキスト間の関係

```text
画像受付・解析
  検証済み解析結果
          |
          v
       家計簿
          ├── 支出
          └── 月次集計（読み取りモデル）

認証 -------- 認証済み利用者ID --------> 各コンテキスト
```

解析結果から支出を作成するときは、解析側の型を家計簿側へ直接持ち込まず、application層(`analysis/application`)で家計簿側の支出集約へ変換する。

## 集約

### 解析依頼集約

解析依頼は、1枚のレシート画像について、受付から終端状態までの整合性を守る集約ルートである。

概念上の属性:

```text
解析依頼
  解析依頼ID
  利用者ID
  レシート画像の参照
  状態
  現在の試行番号
  アップロード期限
  登録された支出ID（登録完了時のみ）
  失敗理由（解析失敗時のみ）
  作成日時
  更新日時
```

主な振る舞い:

```go
request.StartAnalysis(attempt, now)
request.Complete(attempt, expenseID, now)
request.MarkNoData(attempt, now)
request.Fail(attempt, failure, now)
request.Retry(now)
```

保証するルール:

- 最初の解析試行番号は1とする。
- 状態を変更できるのは、指定された試行番号が現在の試行番号と一致する場合だけとする。
- アップロード待ちまたは同一試行の解析中から解析を開始できる。
- 登録完了、登録対象なし、解析失敗は解析試行の終端状態とする。
- 登録完了には支出IDが必要である。
- 解析失敗には失敗理由が必要である。
- 再解析時は試行番号を1増やし、以前の失敗情報を消去する。
- 古い試行の完了、失敗、登録対象なしは現在の状態へ反映しない。
- アップロード期限切れと解析停滞は、時刻から判定する派生状態とし、永続状態を増やさない。

DynamoDBの条件付き更新は、並行処理下でこのルールを保証する永続化上の仕組みとして残す。ドメインモデルによる検証だけで排他制御を代替しない。解析中の状態遷移(`MarkAnalyzing` / `MarkFailed` / `MarkNoData`)と再解析(`MarkRetrying`)は条件付き更新で行い、集約は登録時の生成と読み出し時の復元(状態と付随する値の整合の検証)に使う。

解析中の依頼を再解析可能とする条件、および停滞と判定する時間は未決定である。決定するまでは、特定の時間条件を解析依頼集約の不変条件として固定しない。

### 支出集約

支出は、購入、取引、請求、返金などのうち家計へ金額上の影響を与えるものと、その支出明細の整合性を守る集約ルートである。

概念上の属性:

```text
支出
  支出ID
  利用者ID
  元となった解析依頼ID（解析から作成された場合）
  店名
  購入日
  読取金額
  調整額
  計上額
  支出明細
  編集済みか
```

主な振る舞い:

```go
expense.ChangeStoreName(name)
expense.ChangePurchaseDate(date)
expense.AdjustAmount(amount)
expense.RenameDetail(detailID, name)
expense.ChangeDetailAmount(detailID, amount, quantity)
expense.ChangeDetailCategory(detailID, category)
```

保証するルール:

- 初期構成では、画像解析から得る読取金額は1円以上10,000,000円以下とする。
- 調整額は符号付きとし、減額は負数、増額は正数で表す。
- 計上額は `読取金額 + 調整額` から導出し、外部から直接設定しない。
- 返金を表現できるように計上額は負数を許容する。
- 店舗側の値引きや税は読取金額に反映済みとして扱い、家計簿側で推測または再計算しない。
- 明細が0件の場合は支出を作らず、解析依頼を登録対象なしとする。
- 購入日は実在する暦日とする。
- 支出明細は最大50件とする。
- 明細金額は0円以上10,000,000円以下とする。
- 数量は1以上999以下とする。
- 明細名を空にしない。
- カテゴリはユビキタス言語で定めたカテゴリのいずれかとする。
- 利用者がカテゴリを変更した場合、カテゴリ決定元を利用者に変更する。

解析結果の検証と支出の不変条件は区別する。たとえば「購入日が5年より前なら解析失敗」は解析結果を自動登録してよいか判断するルールであり、利用者が支出を編集するときの購入日制限と同一とは限らない。

### 月次集計

月次集計は、家計簿コンテキストに属し、利用者と対象月で一意になる読み取りモデルである。

概念上の属性:

```text
月次集計
  利用者ID
  対象月
  計上額合計
  支出数
  明細数
  カテゴリ別金額
  バージョン
```

月次集計は次の2通りで更新する。

- 支出登録時に、トランザクション内で原子的に加算する。
- 支出編集時または復旧時に、保存済みの支出と支出明細から再構築する。

月次集計の値を支出へ逆反映しない。集計に不整合がある場合は、支出を正本として再構築する。

## 値オブジェクト候補

実装時には必要になったものから導入し、単なる型の置き換えを目的に一括導入しない。

| 値オブジェクト | 責務 |
| --- | --- |
| `ExpenseID` | 支出IDが空でないことを保証する(`common/domain`) |
| `AnalysisRequestID` | 解析依頼IDが空でないことを保証する(`common/domain`) |
| `Attempt` | 1以上の解析試行番号を表し、次の試行番号を作る(`analysis/domain`) |
| `PurchaseDate` | 時刻を含まない実在する購入日を表す(`common/domain`) |
| `YearMonth` | 有効な年と月を表し、購入日から生成する(`common/domain`) |
| `ReadAmount` | レシートから読み取った最終的な支払合計と許容範囲を表す(`common/domain`) |
| `AdjustmentAmount` | 利用者が加減する符号付きの調整額を表す(`ledger/domain`) |
| `RecordedAmount` | 読取金額と調整額から導出される計上額を表す(`ledger/domain`) |
| `Category` | 定義済みカテゴリ(`social`を含む)だけを表す(`common/domain`) |
| `FailureReason` | 解析失敗コードと安全に表示できる理由を表す(`analysis/domain`) |

## 外部境界のモデル

次のモデルはドメインモデルと分離する。

### OpenAIレスポンス

OpenAI固有のJSON Schema、`refusal`、`incomplete`などを表すinfrastructureのモデル(`receiptOutput`)とする。infrastructureはそれを検証前の読み取り内容(`ReceiptReading`)へ変換し、domainの`ValidateReading`が業務上の検証を行って検証済みの解析結果(`AnalysisResult`)を返す。

```text
OpenAIレスポンス
       |
       | infrastructureで解釈
       v
読み取り内容(ReceiptReading)
       |
       | domainで検証(ValidateReading)
       v
解析結果(AnalysisResult)
       |
       | applicationで支出集約へ変換
       v
支出(Expense)
```

### 永続化モデル

DynamoDBの属性名、PK、SK、GSI用属性、marshal用タグを持つモデルはinfrastructureへ置く。ドメインモデルはテーブル構造や検索用キーを持たない。

### APIモデル

HTTP request/response、パスパラメータ、HTTPステータスは`cmd/<lambda>`の境界でapplicationのinput/outputへ変換する。APIのキーもユビキタス言語に合わせる(`expense_id`、`analysis_request_id`、`purchase_date`、`read_amount` / `adjustment_amount` / `recorded_amount`)。

## 実装の現状

| 項目 | 状態 |
| --- | --- |
| OpenAIレスポンスと解析結果の分離 | infrastructureの`receiptOutput` → `ReceiptReading` → `AnalysisResult`で分離済み |
| 支出モデル | `ledger/domain.Expense`が登録(analyze-receipt)と参照(get-expense)の両方で使われる |
| 解析依頼の状態と実行結果 | `AnalysisStatus`(状態)と`AnalysisOutcome`(解析試行の結末)で区別済み |
| 値オブジェクト | 購入日、対象月、金額、数量、カテゴリを`common/domain`の値オブジェクトで表現済み |
| 状態遷移 | 集約に遷移規則を持ち、永続化はDynamoDBの条件付き更新で並行処理時の整合を保証する。解析中の再解析条件は未決定のまま(集約の`Retry`は`ANALYZING`を受け付けず、再解析APIは停滞した`ANALYZING`もやり直せる) |
| 金額 | `read_amount` / `adjustment_amount` / `recorded_amount`で保存し、`計上額 = 読取金額 + 調整額`で導出する |

## 今後の導入順

1. 支出編集(`PATCH /expenses/{expense_id}`)を支出集約の編集操作で実装する。
2. 月次再構築(`POST /monthly-summaries/{yyyy-MM}/rebuild`)を`RebuildMonthlySummary`で実装する。
3. 解析中の解析依頼を再解析可能とする条件と、停滞と判定する時間を決め、集約の`Retry`と再解析APIの条件を揃える。
