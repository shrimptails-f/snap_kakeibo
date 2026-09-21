# Issue / PR テンプレ

実例: Issue #40、PR #41(upload)。#24 / PR #39(analyze-receipt)。

## Sub-issue

```markdown
## 目的

`cmd/<lambda>/main.go` を #24(analyze-receipt)/ #40(upload)で決めた型に合わせて feature パッケージに分割し、`internal/library` の共通基盤に載せ替える。

現状は <lambdawrap / s3 / sqs の適用状況、DynamoDB は生 SDK か、設定の読み方、認証の方式> 。

## 内容

### 1. `main.go` を分割する

想定する置き場(名前は実装時に調整してよい):

- `internal/<feature>/application`: `<Usecase>Usecase`。<何をして、何を interface で受けるか>
- `internal/<feature>/domain`: <entity と不変条件>
- `internal/<feature>/infrastructure`: <DynamoDB の操作と条件式、S3 / SQS の実装>
- `internal/<feature>/library/settings`: `oswrapper` で起動時に検証する設定(<必須の環境変数>)
- `internal/di/<lambda>.go`: 共通コンテナに <lambda> 固有の依存を追加する
- `cmd/<lambda>/main.go`: 設定読み込み・DI・HTTP 入出力の変換だけ

### 2. 共通基盤への載せ替え

- 認証は `di.provideAccessTokenVerification` + `auth/application.CheckUsecase`(旧 `internal/auth.Service` をやめる)
- DynamoDB は `library/dynamodb` の `Table.<操作>` を使い、`dynamodb_<操作>` span を出す
- `time.Now()` は `timewrapper.Interface`<、ID 採番は `library/ulid`> から受け取る
- handler 内の `app.Required` をやめ、必須の環境変数が無ければ起動時に落とす
<- logger / lambdawrap / awsconfig を導入する(未導入の Lambda の場合)>

### 3. テスト

- `internal/<feature>/application`: <stub で検証する分岐>
- `internal/<feature>/infrastructure`: Floci の DynamoDB に対する結合テスト(`STAGE` が local / ci のときだけ動く)。<条件式が効くこと>
- `internal/di`: コンテナからユースケースを解決できることを確認する

## やらないこと

- 他の Lambda(別 Sub-issue)
- API のリクエスト / レスポンス形式の変更
- `<table>` の項目・キー構成の変更

## 完了条件

- `main.go` が配線だけになっている
- `go test -race ./...` が通り、Floci の結合テストが CI でも動く
- dev にデプロイし、`invocation_started` / `<span 名>` span / `invocation_finished` が 1 つの `trace_id` で繋がっている

Parent: #23
```

## PR 本文

```markdown
Closes #<NN>(親: #23)

## 概要

`cmd/<lambda>/main.go` を #24 / #40 と同じ型で feature パッケージに分割し、`internal/library` の共通基盤に載せ替えました。

## 構成

```
cmd/<lambda>/main.go             設定読み込み・DI・HTTP 入出力の変換だけ
internal/<feature>/
├── application/   ...
├── domain/        ...
├── infrastructure/
│   ├── ...
│   └── keys.go
└── library/settings/  ...
internal/di/<lambda>.go
```

### 共通基盤側の変更

- (あれば。無ければ「なし」)

### 旧実装からの差分

- 認証は旧 `internal/auth.Service` から `auth/application.CheckUsecase` + `token.JWTVerifier` に変更(HS256 / issuer / 期限の検証は同じ)
- DynamoDB は生 SDK から `library/dynamodb` に変更し、`dynamodb_<操作>` span が出る。ctx に `user_id` / `analysis_request_id` を積むので span にも付く
- handler 内の `app.Required` をやめ、必須の環境変数が無ければ起動時に落とす
- <挙動が変わる点があれば明示。無ければ「API のリクエスト / レスポンス形式と DynamoDB の項目は変更なし」>

## テスト

- `internal/<feature>/application`: ...
- `internal/<feature>/domain`: ...
- `internal/<feature>/infrastructure`: Floci の DynamoDB に対する結合テスト(ランダム名のテーブル、`STAGE` が local / ci のときだけ動く)。...
- `internal/di`: ...

`gofmt` / `STAGE=local go test -race ./...` / `go vet` / `go mod tidy` / golangci-lint v2.11.3 を通過。

## dev での確認

(deploy/dev にマージしてログを確認したら、確認できた span の連なりを書く。未確認なら「デプロイ後にログで確認が必要」)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```
