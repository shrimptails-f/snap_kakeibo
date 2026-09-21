# CodePipeline 設計

## 採用方針

デプロイ単位が違うため、backend / frontend / infra の CodePipeline は分ける。

```text
backend pipeline
  GitHub -> BuildAndDeploy(CodeBuild: test -> ECR push -> Lambda publish -> CodeDeploy -> marker)

frontend pipeline
  GitHub -> BuildAndDeploy(CodeBuild: test/build -> S3 sync -> CloudFront invalidation -> marker)

infra pipeline
  TODO
```

backend と frontend を同じ pipeline に入れると、片方の失敗がもう片方のデプロイや成功 marker 更新に影響する。アプリケーションとしては後方互換性を前提にし、backend と frontend は独立してデプロイできる構成にする。

## Pipeline 一覧

| pipeline | 対象 | 成功 marker | 方針 |
| --- | --- | --- | --- |
| `backend` | Lambda コンテナイメージ / Lambda Version / CodeDeploy | `/{stage}/snap-kakeibo/cicd/backend/last-successful-commit` | Lambda を差分デプロイする |
| `frontend` | React build / S3 / CloudFront | `/{stage}/snap-kakeibo/cicd/frontend/last-successful-commit` | front の差分があるときだけ配信する |
| `infra` | CDK / CloudFormation | `/{stage}/snap-kakeibo/cicd/infra/last-successful-commit` | TODO。まずは手動 `cdk deploy` を継続する |

GitHub 接続は pipeline 間で共有する。

```text
/{stage}/snap-kakeibo/cicd/github-connection-arn
```

## デプロイ対象ブランチ

デプロイ対象ブランチは backend / frontend で分けず、stage ごとに共有する。

```text
deploy/dev -> dev
deploy/stg -> stg
deploy/prd -> prd
```

backend と frontend は同じモノレポ内の同じプロダクトなので、「この commit をこの stage に出す」という意思決定は stage ごとの deploy branch で一元化する。

```text
dev backend pipeline   watches deploy/dev
dev frontend pipeline  watches deploy/dev

stg backend pipeline   watches deploy/stg
stg frontend pipeline  watches deploy/stg

prd backend pipeline   watches deploy/prd
prd frontend pipeline  watches deploy/prd
```

backend / frontend の独立性は branch ではなく、pipeline 分離、成功 marker 分離、差分検出、後方互換性で担保する。

通常の昇格は deploy branch を進めることで表す。

```bash
git checkout deploy/dev
git merge main
git push origin deploy/dev

git checkout deploy/stg
git merge deploy/dev
git push origin deploy/stg

git checkout deploy/prd
git merge deploy/stg
git push origin deploy/prd
```

stage 名は dev / stg / prd で統一し、ブランチ名・AWS 上のリソース名・SSM prefix・Lambda の `STAGE` 環境変数で同じ値を使う。

## SSM Parameter

pipeline が更新する運用状態は CDK の `StringParameter` として値を管理しない。既存の Lambda image tag parameter と同じく、`ensure-parameters` で「無ければ作る、既存値は上書きしない」形にする。

必要な SSM parameter:

```text
/{stage}/snap-kakeibo/cicd/github-connection-arn
/{stage}/snap-kakeibo/cicd/backend/last-successful-commit
/{stage}/snap-kakeibo/cicd/frontend/last-successful-commit
/{stage}/snap-kakeibo/cicd/infra/last-successful-commit
```

初期値は `UNSET` とする。

`github-connection-arn` は事前に AWS CodeConnections / CodeStar Connections で作った connection ARN を手動設定する。`UNSET` のまま Pipeline stack をデプロイしようとした場合は失敗させる。

Connection は Pipeline stack と同じリージョンに作る。現行の dev 環境は `ap-northeast-2` なので、`arn:aws:codeconnections:ap-northeast-2:...` の ARN を設定する。`ap-northeast-1` など別リージョンの connection ARN は使わない。

現行実装では CDK が `dev-snap-kakeibo-pipeline` に backend / frontend の 2 本の CodePipeline を作る。各 pipeline は `Source` / `BuildAndDeploy` stage に分ける。Source action は `CodeBuildCloneOutput` を有効にし、BuildAndDeploy stage の CodeBuild 内で `.git` を使えるようにする。これは `git diff --name-status base head` と `go list -deps` による差分判定に必要。

```bash
task infra:parameters
aws ssm put-parameter \
  --name /dev/snap-kakeibo/cicd/github-connection-arn \
  --type String \
  --value arn:aws:codeconnections:ap-northeast-2:<account-id>:connection/<connection-id> \
  --overwrite
task infra:deploy:pipeline
```

## 差分検出

各 pipeline は自分の成功 marker と今回の source revision を比較して、対象差分を判定する。

```text
base = SSM last-successful-commit
head = CodePipeline source revision

if base == UNSET:
  deploy all for this pipeline
else:
  git diff --name-status base head
```

marker は pipeline ごとに分ける。backend が成功して frontend が失敗した場合でも、backend marker だけを更新できる。

差分は `--name-status` で取得する。追加・変更だけでなく、削除(`D`)や rename(`R`)を検出するため。

## Backend Pipeline

### 対象

`backend/cmd/*` の全 Lambda を対象にする。API Gateway 同期呼び出しの関数も SQS 起動の `analyze-receipt` も、呼び出し元を `live` Alias に向けて CodeDeploy で切り替える(経緯は `deployment-strategy.md` の「対象 Lambda」を参照)。

```text
hello
upload
retry-analysis
list-analysis-requests
get-expense
analyze-receipt
```

対象関数の一覧はスクリプトに列挙せず、`backend/cmd/*/` から求める。関数を足すときは AppStack 側でも `CodeDeploy: true` にして `live` Alias を作る。

### 差分ルール

backend は単純な path prefix だけではなく、Go の package 依存関係で影響範囲を判定する。方針は `codepipeline_monorepo_practice` と同じく、Git 差分と `go list -deps` を組み合わせる。

| 変更 | deploy 対象 |
| --- | --- |
| `backend/cmd/{function}/**` | 対応する関数 |
| `backend/internal/**` の既存 Go package | その package に依存する Lambda |
| `backend/go.mod`, `backend/go.sum` | Lambda 全て |
| `backend/Dockerfile` | Lambda 全て |
| `scripts/` の backend deploy 関連 | Lambda 全て |
| `infra/` | backend pipeline では扱わない。infra pipeline の TODO |
| 削除、rename、判定不能な共有 package | Lambda 全て |
| docs のみ | deploy なし |
| test file のみ | test は実行するが deploy 対象にはしない |

初回や marker が `UNSET` の場合は全 backend 関数の ECR image を push し、全関数を CodeDeploy で deploy する。

### Package 依存判定

Lambda ごとに `go list -deps` で依存 package の集合を作る。

```bash
cd backend
go list -deps -f '{{.ImportPath}}' ./cmd/hello
go list -deps -f '{{.ImportPath}}' ./cmd/upload
go list -deps -f '{{.ImportPath}}' ./cmd/retry-analysis
go list -deps -f '{{.ImportPath}}' ./cmd/list-analysis-requests
go list -deps -f '{{.ImportPath}}' ./cmd/get-expense
go list -deps -f '{{.ImportPath}}' ./cmd/analyze-receipt
```

変更された `.go` ファイルのディレクトリを package として解決し、各 Lambda の依存集合と照合する。

```text
changed package ∩ function dependencies != empty
  -> deploy that function
```

これは関数・メソッド単位ではなく package 単位の判定とする。見逃しを避ける代わりに、同じ package 内の未使用関数だけを変更した場合でも deploy 対象になることがある。

### 削除と rename

削除された Go ファイルや rename された package は、現在の worktree だけでは旧 package の import path や旧依存関係を `go list` で確認できない。

初期実装では、次の変更は安全側に倒して Lambda 全てを deploy する。

```text
backend/internal 配下の Go ファイル削除
backend/internal 配下の package rename
backend/cmd 配下の function directory 削除または rename
判定スクリプトが package を解決できない変更
```

将来精度を上げる場合は、`base` と `head` の両方の worktree で依存グラフを作り、旧依存と新依存の和集合で影響範囲を判定する。

```text
affected functions =
  functions depending on changed package at base
  ∪ functions depending on changed package at head
```

この方法なら削除された package に依存していた Lambda も検出できる。

### 実行内容

backend pipeline は BuildAndDeploy stage の CodeBuild で次を実行する。

```text
1. テストを実行する
2. 差分から対象 Lambda を判定する
3. 対象 Lambda image を ECR へ push する
4. 対象 Lambda 一覧、image URI、関数名、Deployment Group 名を backend-plan.json に書く
5. live Alias が既に今回の image URI を使っていれば skip する
6. aws lambda update-function-code --publish で新 Version を発行する
7. live Alias の現在 Version を取得する
8. Lambda AppSpec を生成する
9. aws deploy create-deployment を実行する
10. deployment 成功を待つ
11. live Alias が新 Version を向いたことを確認する
12. 全対象が成功したら backend marker を head commit に更新する
```

CodeDeploy Application / Deployment Group / Deployment Config / Lambda Alias は AppStack が作る。

実行ロジックは `scripts/backend-build.sh` と `scripts/backend-deploy.sh` に置く。初回または marker が `UNSET` の場合は `backend/cmd/*` 全関数の ECR image/tag を揃え、全関数の CodeDeploy deployment を作る。通常時は `git diff --name-status` で削除・rename を検出し、`backend/internal` の既存 Go package 変更は `go list -deps` の依存集合で対象 Lambda に絞る。

再実行時は、同じ commit tag の ECR image が既に存在すれば build / push を skip する。さらに `live` Alias が既に今回の image URI を使っている Lambda は、Lambda Version 発行と CodeDeploy deployment も skip する。これにより、一部 Lambda の CodeDeploy 成功後に後続 Lambda で失敗した場合でも、再実行で成功済み Lambda を重複 publish せず、未完了分だけ進められる。backend marker は全対象が成功した最後にだけ更新する。

## Frontend Pipeline

### 差分ルール

| 変更 | deploy 対象 |
| --- | --- |
| `front/**` | frontend |
| frontend deploy 関連 script | frontend |
| `infra/` | frontend pipeline では扱わない。infra pipeline の TODO |

初回や marker が `UNSET` の場合は frontend をデプロイする。

### 実行内容

frontend pipeline も BuildAndDeploy stage の CodeBuild で marker と差分を見て、配信が必要な場合だけ次を実行する。

```text
1. npm ci / npm run build を実行する
2. front/dist を frontend bucket へ sync する
3. CloudFront invalidation を作成する
4. 成功したら frontend marker を head commit に更新する
```

実行ロジックは `scripts/frontend-build.sh` と `scripts/frontend-deploy.sh` に置く。`front/**` または frontend deploy script に差分がある場合、または marker が `UNSET` の場合だけ配信する。差分がない場合は build/sync/invalidation を行わず marker だけ更新する。

CloudFront invalidation の完了待ちは初期実装では必須にしない。必要になったら `wait invalidation-completed` を追加する。

## Infra Pipeline

infra pipeline は TODO とする。

当面は次の手動操作を継続する。

```text
task infra:diff
task infra:deploy:storage
task infra:deploy:app
```

将来 infra pipeline を作る場合は、アプリ deploy pipeline とは分ける。CDK deploy は Lambda image push / frontend sync とは責務が違い、失敗時の影響範囲も大きいため。

## 後方互換性

backend と frontend は別 pipeline で独立して進むため、backend は古い frontend と動く後方互換性を保つ。

破壊的変更は 1 回の deploy にまとめない。

```text
1. backend に新旧両対応を入れる
2. frontend を新仕様へ移す
3. backend から旧仕様を消す
```

必要に応じて feature flag を使い、新 backend / 新 frontend の有効化タイミングを deploy から切り離す。
