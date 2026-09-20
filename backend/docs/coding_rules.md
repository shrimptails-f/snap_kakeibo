# Backend コーディング規約

## コンテキスト

- Language: Go
- Runtime: AWS Lambda（コンテナイメージ）
- Architecture: feature package を単位とした Clean Architecture
- DI: `go.uber.org/dig`
- AWS SDK: AWS SDK for Go v2

本書は `backend/` の新規実装と既存コードの変更に適用する。既存コードと規約が食い違う場合は、変更範囲で無理に全面改修せず、意図を確認して段階的に揃える。

## 基本原則

- SOLID、SRP、YAGNI、DRY、KISS を尊重する。
- `cmd/<lambda>/main.go` は設定読み込み、クライアント生成、DI、ハンドラ登録、HTTP 入出力の変換に留める。
- 業務判断とユースケースのオーケストレーションは `internal/<feature>/application` に置く。
- feature 固有の値と不変条件は `internal/<feature>/domain`、複数 feature で共有するものは `internal/common/domain` に置く。
- DynamoDB など外部システムの実装は `internal/<feature>/infrastructure` に置く。
- SDK や汎用技術の wrapper は `internal/library` に置き、上位層が必要とする最小限の interface を application 側で定義する。
- 変更可能な package-level グローバル変数は避ける。Lambda の起動時に一度だけ構築する handler 依存、定数、センチネルエラーは許容する。
- 実行時の想定可能な失敗は `error` で返す。`panic` は起動時初期化の失敗や回復不能な不変条件違反に限定する。

## ディレクトリと依存方向

feature package は必要に応じて次の構成にする。

```text
internal/<feature>/
├── application/    # usecase、入出力、依存 interface、application error
├── domain/         # entity、value object、不変条件
├── infrastructure/ # DynamoDB など interface の実装
└── library/        # feature 内で閉じる token、cookie などの技術要素
```

- `application` は infrastructure の具象型や AWS SDK 型へ依存しない。
- `infrastructure` と `library` は application が定義した interface を実装する。
- HTTP request/response や Lambda event の型を application/domain へ持ち込まない。
- package 間の循環依存を作らない。共通化は実際に複数箇所で必要になってから行う。

## 命名

- 変数と非公開識別子は camelCase、公開識別子は PascalCase とする。
- 初期化関数は `NewXxx`、取得は `FindBy...` / `Get...`、保存は `Save...`、失効は `Revoke...` など、振る舞いが分かる動詞を使う。
- application の入出力は `<Usecase>Input` / `<Usecase>Output` とする。
- HTTP 層が依存する契約は既存コードに合わせて `<Usecase>UsecaseInterface` とする。
- repository や外部機能の interface は利用側の能力を表す名前にする（`UserRepository`、`RefreshTokenFinder`、`AccessTokenVerifier` など）。
- import alias は衝突回避やレイヤーの明確化に必要な場合だけ使い、`authdomain`、`libdynamodb` のように意味が分かる lowerCamelCase とする。
- `me` のように主体が曖昧な名前を避け、`check`、`currentUser` など目的が分かる名前を選ぶ。

## 実装ルール

### Context

- I/O または呼び出しのキャンセルが関係する公開メソッドは、第1引数に `context.Context` を受け取る。
- request の context を下位層まで渡し、理由なく `context.Background()` に置き換えない。
- 長寿命 goroutine に request context をそのまま保持しない。
- context に格納する値は request ID、trace、user ID など相関に必要な最小限とする。

### 設定と時刻

- 環境変数は `oswrapper.Interface` 経由で起動時に読み込み、必須値を検証する。handler や application から `os.Getenv` を直接呼ばない。
- stage の判定は `internal/library/stage` を利用する。
- 現在時刻に依存する処理は `timewrapper.Interface` を注入し、テストで固定可能にする。
- secret は環境変数または SSM の provider に閉じ込め、application へ取得方法を漏らさない。

### エラーハンドリング

- 呼び出し側が分岐すべき失敗は application/domain のセンチネルエラーまたは型付きエラーで表す。
- 原因を保持する必要がある場合は `fmt.Errorf("operation: %w", err)` でラップする。
- 認証情報の存在などを推測できる詳細を外部レスポンスへ出さない。
- HTTP ステータスへの変換は `cmd/<lambda>` の handler 境界で行い、application は HTTP ステータスを返さない。
- エラーを握りつぶす場合は、冪等性など業務上成功として扱える理由をコメントまたはテストで明確にする。

### DynamoDB

- table 名の束縛と SDK 呼び出しは `internal/library/dynamodb` を使う。
- feature 固有のキー構築、attribute 変換、repository は feature の infrastructure に置く。
- 条件付き書き込みや一意性はアプリケーション側の事前確認だけに頼らず、DynamoDB の condition expression で保証する。
- token、メールアドレスなどをキーへ含める場合は、保存形式と漏えいリスクを確認する。

### ドキュメントコメント

- package 外から利用する型、関数、メソッド、センチネルエラーには、目的や契約が分かる Go doc コメントを付ける。
- コメントは処理の逐語訳ではなく、理由、境界条件、呼び出し側が守る契約を記述する。

## ロギング

`internal/library/logger` の現行実装を標準とする。

- ログ出力は `logger.Interface` を経由し、標準 `log` package や `fmt.Printf` を業務コードで使わない。
- Lambda handler は `lambdawrap.Handle` / `HandleEvent` を使い、request ID、trace、開始・終了、panic recovery の共通処理に載せる。
- 追加情報は `logger.Field` と定義済み helper を使って構造化する。
- context の相関情報は logger の context helper を利用し、文字列へ埋め込まない。
- Authorization ヘッダー、JWT、refresh token、Cookie、password、secret、API key はログへ出さない。
- メールアドレスなどの個人情報は原則として出さず、調査に必要な場合も識別子や安全なメタデータを優先する。
- 同じエラーを各レイヤーで重複して記録しない。原則として、処理境界で必要な文脈を付けて1回記録する。
- `panic` の値やエラー文字列に秘匿情報を含めない。logger の redaction を過信せず、入力値そのものを field に渡さない。

## テスト

- テストファイル名は `xxx_test.go`、テスト関数名は `TestXxx` とする。
- 原則として1テストで1つの振る舞いを検証し、複数パターンはテーブルドリブンテストを検討する。
- stub、fake、mock は可能な限りテストファイル内に置き、依存を局所化する。
- application テストでは AWS SDK を直接使わず、application が定義する interface の stub を使う。
- 時刻境界は `timewrapper.NewFixed` などで固定し、有効期限の直前・一致・超過を明示的に検証する。
- エラーは文字列一致より `errors.Is` / `errors.As` を優先する。

### 並列実行

- 独立した単体テストはテスト関数の先頭で `t.Parallel()` を呼ぶ。
- 並列サブテストでは、range 変数や mutable な fixture をテスト間で共有しない。
- 環境変数やファイルを読むコードの利用側テストでは、`oswrappertest.Mock` などの依存を注入し、`t.Setenv`、`os.Setenv`、`t.Chdir` による process-global state の変更を避ける。
- OS wrapper 自体の境界テストなど、process-global state の操作が検証対象で代替できないものだけ直列実行する。
- Floci その他の共有外部サービスを使う統合テストは、テストごとの Nano ID を付けた一時リソースを作成し、`t.Cleanup` で自分のリソースだけを削除する。リソースとデータが分離できれば並列実行する。
- cleanup の順序に依存して後処理を検証する場合は、独立した subtest の終了を境界にするか cleanup 関数を明示的に呼び、他テストとの実行順へ依存させない。
- 変更可能な package-level state が必要な設計は、可能なら instance へ閉じ込める。外部仕様として共有状態そのものを検証するテストだけ直列実行する。
- 並列化後は `go test -race ./...` を実行し、race detector を通す。
- `t.Parallel()` を件数合わせで追加しない。安全性を説明できないテストは直列のままにする。

## 変更時の確認

最低限、変更範囲に応じて次を実行する。

```sh
cd backend
gofmt -w <変更した Go ファイル>
go test -race ./...
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
```

- Lambda 名を追加・変更した場合は `backend/cmd/<name>` と `infra/common.FunctionNames` を一致させる。
- API パス変更では backend、infra、frontend、テスト、ドキュメントを横断検索する。
- public API、設定値、DynamoDB schema を変更した場合は互換性とデプロイ順を確認する。
