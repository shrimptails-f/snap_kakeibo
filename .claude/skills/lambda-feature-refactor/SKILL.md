---
name: lambda-feature-refactor
description: backend/cmd/<lambda>/main.go を internal/<feature> の application / domain / infrastructure / library に分割し、internal/library の共通基盤(logger / lambdawrap / dynamodb / s3 / sqs / oswrapper / timewrapper / ulid)と DI に載せ替えるための手順書。Issue #23 の Sub-issue 作成 → ブランチ → 実装 → 検証 → PR → deploy/dev → ログ確認 → #23 更新 までを一気通貫で扱う。「〜を feature パッケージに分割して」「retry-upload / list-uploads / get-billing を対応して」「#23 の残りをやって」「Lambda を共通基盤に載せ替えて」「新しい Lambda を追加して」のように backend の Lambda の構成に触れる依頼では、明示されなくても必ずこのスキルを読んでから着手する。
---

# Lambda を feature パッケージに分割する

`backend/cmd/<lambda>/main.go` に業務ロジックが直書きされている Lambda を、
`backend/docs/directory-structure.md` の構成に分割し、共通基盤に載せ替える作業の手順書。
親 Issue は #23。コードの規約は `backend/docs/coding_rules.md` が正で、ここには再掲しない。
このスキルが持つのは「どの順で何を見て、何を揃え、どこでハマるか」。

## 0. 前提

- GitHub 操作は `source "$(git rev-parse --show-toplevel)/scripts/gh-wrappers.bash"` してから `ghp`(AGENTS.md)。
- Issue / PR / コミット / 会話は日本語。コミット末尾の Co-Authored-By は session の指示に従う。
- Floci が `AWS_ENDPOINT_URL` で立っている devcontainer 前提。結合テストは `STAGE=local` で動く。

## 1. 準備: 型を把握する(実装前に必ず読む)

読む順番と、そこから取るもの:

| 読むもの | 取るもの |
| --- | --- |
| `ghp issue view 23 --json body -q .body` | 対象の状態表、「残りの Lambda で揃えること」 |
| `backend/cmd/<lambda>/main.go` | 今の入出力、DynamoDB / S3 / SQS 操作、認証の有無、HTTP ステータスの対応 |
| `backend/internal/upload/` 一式 + `backend/internal/di/upload.go` | **HTTP API の型**(認証あり・DynamoDB 1 回・S3 presign)。最新なので最優先の参照 |
| `backend/internal/analysis/` 一式 + `backend/internal/di/analyze_receipt.go` | **イベント駆動の型**(SQS → usecase、条件付き UpdateItem、トランザクション、OpenAI) |
| `backend/internal/di/auth.go` | `provideAccessTokenVerification` / `provideAuthTokenDependencies` |
| `backend/internal/library/dynamodb/client.go` の先頭コメント | `Table.GetItem / PutItem / UpdateItem / Query`、`Client.TransactWriteItems`、span 名 |
| `backend/internal/app/` | 旧共通 Config / キー構築。**新コードからは参照しない**(キー形式の一致テストで比較するだけ) |

`internal/upload` と `internal/analysis` を先に読めば、命名・ファイル分割・テストの粒度はほぼそのまま写せる。
判断に迷ったら upload に揃える(後発で、#23 のフィードバックが反映されている)。

## 2. Sub-issue を作る

`references/templates.md` の Issue テンプレで作り、#23 の Sub-issue に紐づける。

```bash
ghp issue create --title "<lambda> を feature パッケージに分割し共通基盤に載せ替える" --body-file <scratchpad>/issue.md
# 紐づけ(GraphQL。ghp issue edit では出来ない)
PARENT=$(ghp api graphql -f query='query{repository(owner:"shrimptails-f",name:"snap_kakeibo"){issue(number:23){id}}}' -q .data.repository.issue.id)
CHILD=$(ghp api graphql -f query='query{repository(owner:"shrimptails-f",name:"snap_kakeibo"){issue(number:<NN>){id}}}' -q .data.repository.issue.id)
ghp api graphql -H "GraphQL-Features: sub_issues" -f query="mutation{addSubIssue(input:{issueId:\"$PARENT\",subIssueId:\"$CHILD\"}){subIssue{number}}}"
```

- `gh` が `HTTP 499` で落ちることがある。作成されていないか `ghp issue list` で確かめてから再試行する(二重作成を避ける)。
- 完了条件に書く span 名は実装に合わせる。presign(`PresignPutObject`)はネットワークに出ないので span を出さない。
  出るのは `dynamodb_*` / `s3_get_object` / `s3_put_object` / `sqs_send` / `openai_request` と、lambdawrap の `invocation_started` / `invocation_finished`。

ブランチは `refactor/<lambda>-feature-package`。

## 3. 実装

### 置き場

```
internal/<feature>/
├── application/   <Usecase>Input / <Usecase>Output、<Usecase>UsecaseInterface、interfaces.go、errors.go
├── domain/        entity と不変条件(HTTP / DynamoDB の都合は持ち込まない)
├── infrastructure/ dynamodb_<resource>_repository.go、s3_*.go、keys.go
└── library/settings/ config.go(oswrapper で起動時検証)
internal/di/<lambda>.go   New<Lambda>Container + Resolve<Lambda>Usecase
cmd/<lambda>/main.go      settings.Load → awsconfig.Load → logger.New → di → Resolve、handler は入出力変換だけ
```

feature 名は既存に合わせる: アップロード系(`upload` / `retry-upload` / `list-uploads`)は `internal/upload`、
請求系(`get-billing`)は `internal/billing` を新設。同じ feature に usecase を足すときは
既存の `interfaces.go` / `errors.go` / `keys.go` に追記し、新しい DI ファイルから既存 provider を再利用する。

### 揃えるもの(#23「残りの Lambda で揃えること」の具体)

- **認証**: `di.provideAccessTokenVerification(container, scope, cfg.JWTSecretParameter, cfg.Stage)` を呼び、
  `di.ResolveAuthCheckUsecase` で `authapp.CheckUsecaseInterface` を取る。handler では
  `check.Check(ctx, authapp.CheckInput{Authorization: header})` → `ErrUnauthorized` なら 401。
  ヘッダは `req.Headers["authorization"]` が空なら `"Authorization"` も見る(API Gateway v2 は小文字化するが、ローカル呼び出しは元のまま)。
  旧 `internal/auth.Service` は使わない。署名鍵は `SSM_JWT_SECRET` のみ(`JWT_SECRET` 分岐は削除済み)。
- **設定**: `internal/<feature>/library/settings.Load(osw)`。必須の環境変数が無ければ `Config{}, err` を返し、`main.init` で panic。
  handler 内の `app.Required` は消す。upload の `config.go` のループ形式に揃える。
- **時刻 / ID**: `timewrapper.Interface`(DI の共通 provider にある)、`ulid.New(clock)` を `application.IDGenerator` として登録。
- **DynamoDB**: `client.Table(cfg.XxxTable)` を repository に持たせる。条件式・キー構築・attribute 変換は infrastructure に閉じる。
  `libdynamodb.IsConditionalCheckFailed(err)` で条件不一致を判定し、application の sentinel error に変換する。
  `ReturnValues` で更新後の値が要るなら UpdateItem の出力を repository で読む(retry-upload の attempt)。
- **キー形式**: `keys.go` に `UserPK` / `UploadSK` などを置き、`keys_test.go` で `app` と `analysis/infrastructure` の関数と一致することを確認する
  (`internal/app` が消えるまでの安全網)。
- **SQS**: `libsqs.Client.Queue(cfg.AnalyzeQueueURL)` を `application` の送信 interface の実装に包む。
  `SendJSON` は ctx の trace を traceparent に載せるので、usecase で `logger.ContextWith(ctx, logger.UploadID(...))` を先に積む。
- **ログ**: usecase の先頭で `logger.ContextWith(ctx, logger.UserID(...), logger.UploadID(...))`。以降の `dynamodb_*` span に自動で付く。
  `http_status_code` は `lambdawrap.Handle` が `invocation_finished` に付けるので個別対応不要。
- **HTTP 変換**: `app.JSON` / `app.Error` は引き続き cmd で使ってよい(`internal/app/http.go` は残す)。
  application の sentinel error → ステータスの対応は handler の `errors.Is` で行う。

### テスト(4 種類、全部書く)

| 場所 | 中身 | 参考 |
| --- | --- | --- |
| `application/<usecase>_test.go` | interface の stub を fixture にまとめ、成功 / 各 sentinel error / 依存の失敗時に後続を呼ばないこと | `upload/application/create_upload_test.go` |
| `domain/*_test.go` | 不変条件、既定値、境界(JST → UTC の月跨ぎなど) | `upload/domain/upload_history_test.go` |
| `infrastructure/*_integration_test.go` | `dynamodbtest.Connect(t)` + `env.CreateTable(t, libdynamodb.XxxSchema)`。書いた属性一式と条件式の拒否を確認。`package infrastructure_test` | `upload/infrastructure/dynamodb_upload_history_repository_integration_test.go` |
| `di/<lambda>_test.go` | コンテナからユースケースが解決できること。SSM は遅延取得なので `JWTSecretParameter: "/test/jwt-secret"` で Floci 不要 | `di/upload_test.go` |

S3 / SQS の infrastructure は `libs3.NewWithAPI(api, presigner, nil)` / `libsqs.NewWithAPI` にフェイクを渡す単体テストにする。
SSM の値が実際に要るテストは `ssmtest.PutSecureString(t, env.Config, prefix, value)`。

## 4. 検証(全部通してから PR)

```bash
cd backend
gofmt -l cmd internal test            # 出力なしが正
go vet ./... && go build ./...
STAGE=local go test -race ./...       # STAGE 無しだと結合テストが Skip になるので必ず付ける
go mod tidy && git diff --exit-code -- go.mod go.sum
# golangci-lint はローカルに無い。CI と同じ版を scratchpad に入れる(版は .github/workflows/_go-module.yml の GOLANGCI_LINT_VERSION)
GOBIN=<scratchpad>/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3
<scratchpad>/bin/golangci-lint run ./...
```

結合テストが本当に走ったかは `-v -run <Test名>` で `--- PASS`(`SKIP` でない)を一度見る。

## 5. PR

`references/templates.md` の PR テンプレ。タイトルは `refactor(backend): <lambda> を feature パッケージに分割し共通基盤に載せ替える`、
本文冒頭は `Closes #<NN>(親: #23)`。

- `ghp pr edit` は Projects classic 廃止の GraphQL エラーで落ちる。本文の更新は
  `ghp api -X PATCH repos/shrimptails-f/snap_kakeibo/pulls/<N> -F body=@<file>` を使う。
- 後から追加した変更(共通基盤側の修正など)は別コミットにして、PR 本文の「共通基盤側の変更」に追記する。

## 6. deploy/dev で確認

ユーザーが「deploy/dev に出して」と言ったら、PR のマージを待たずに feature ブランチをマージして push する
(`docs/ci_cd/codepipeline-design.md`。deploy/dev は main 相当なので通常 fast-forward)。

```bash
git checkout deploy/dev && git pull --ff-only origin deploy/dev
git merge --no-edit refactor/<lambda>-feature-package && git push origin deploy/dev
git checkout refactor/<lambda>-feature-package
```

ログの確認観点(ユーザーが CloudWatch のログを貼ってくる):

- `invocation_started` → `<span> started / finished` → `invocation_finished` が同じ `trace_id` / `request_id`
- span の `parent_span_id` が invocation の `span_id`
- span に `user_id` / `upload_id` / `table_name` が付いている
- `invocation_finished` に `http_status_code` がある(API の場合)
- span が無ければ handler が 401 / 400 で早期 return している。`http_status_code` で切り分ける

## 7. 後始末

- ユーザーの指示で PR をマージしたら(`ghp pr merge <N> --merge --delete-branch`。CI の `go / build-test` と `go / lint` が通っていることを先に見る)、#23 の状態表を更新する
  (対象行を `✅ 完了(#NN / PR #MM)` にし、残りの行の「現状」を最新にする)。
- `retry-upload` / `list-uploads` / `get-billing` の 3 本がすべて終わったら、`internal/auth/auth.go`、`internal/app/config.go`、
  `internal/app/model.go` と各 `keys_test.go` の `app` 比較を削除する PR を別に立てる。
- Lambda を新規追加した場合は `infra/common.FunctionNames` と `infra/stacks/app.go` の権限(GrantXxx / grantJWTSecretRead)も揃える。
