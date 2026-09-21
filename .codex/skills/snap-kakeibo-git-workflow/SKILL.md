---
name: snap-kakeibo-git-workflow
description: snap_kakeibo リポジトリで Issue、ブランチ、commit、Pull Request、マージ、deploy ブランチへの反映を行う。Issue / PR 本文と commit message の書き方、親子 Issue の紐付け、PR 作成前の検証と報告が対象。コードの実装規約は各実装スキル(snap-kakeibo-backend / snap-kakeibo-frontend)に従う。
---

# snap_kakeibo Git / GitHub 運用

## 着手前

1. リポジトリルートの `AGENTS.md` を読む。GitHub の操作は `ghp`(このリポジトリ)/ `ghw`(他リポジトリの参照)で行い、`gh` を直接実行しない。
2. `git status` と `git log --oneline -10` で現在のブランチと直近の履歴を確認する。
3. 既存の Issue / PR を数件読み、粒度と文体を揃える。

```sh
source "$(git rev-parse --show-toplevel)/scripts/gh-wrappers.bash"
ghp issue list -L 10 --state all
ghp pr list -L 5 --state merged --json number,title,body
```

## 共通の書き方

- すべて日本語で書く。英語の識別子(パス、API、クラス名、コマンド)はバッククォートで囲む。
- 見出しは `## 目的` `## 内容` のように短い名詞にし、本文は箇条書きを基本にする。
- 事実だけを書く。やっていないこと、確認していないことを「済み」と書かない。
- 判断した理由を残す。「なぜこの形にしたか」「参照元・文書のどこに基づくか」を 1 行で添える。
- 文書(`docs/`、`front/docs`、`backend/docs`)を正本とし、文書の節番号(`architecture.md` §7 など)で根拠を示す。
- 利用者から見える振る舞いの変更と、意図的にやらなかったことは必ず別節に分けて明示する。
- リポジトリ名や Issue 番号を推測しない。番号は作成結果の URL から取る。

## Issue

### タイトル

「〜する」で終わる動詞句にする。API や画面が特定できる場合は括弧で補う。

```text
月次集計一覧 API を実装する(GET /monthly-summaries)
フロントエンドの共通関数(HTTP client / 認証セッション / Router)を整える
フロントエンドの開発基盤を整備する
```

### 本文の構成

```markdown
## 目的

何のために、どの文書・画面仕様に基づいて行うか。親 Issue があれば「#67 の小課題。」から始める。
現状の問題(未実装、文書との不一致など)を 1〜2 文で書く。

## 内容

- 変更対象を層(backend / infra / docs、shared / features / app)ごとに小見出しで分ける
- ファイルパスと関数名まで書き、実装者が迷わない粒度にする
- 契約(Request / Response、並び順、制約)は正本の文書を指す

## 利用者から見える変更   ← 画面や API の振る舞いが変わる場合だけ

## やらないこと

- 意図的にスコープ外にしたものと、その理由(別 Issue、導入タイミング未達など)
```

親 Issue(基盤整備や複数 API の束)は `## 小課題` にチェックリストを置き、子 Issue ができたら `- [ ] #68 〜` の形で番号を入れる。

### ラベル

機能追加・基盤整備は `enhancement`、不具合は `bug`、文書のみは `documentation`。

### 作成と親子の紐付け

本文は heredoc で `--body-file -` に渡す。

```sh
ghp issue create --label enhancement --title "..." --body-file - <<'EOF'
...
EOF
```

子 Issue は GitHub の sub-issue として紐付ける。`node_id`(GraphQL の ID)を使う。`id` では失敗する。

```sh
PARENT=$(ghp api repos/OWNER/REPO/issues/67 --jq .node_id)
CHILD=$(ghp api repos/OWNER/REPO/issues/68 --jq .node_id)
ghp api graphql \
  -f query='mutation($p: ID!, $c: ID!) { addSubIssue(input: {issueId: $p, subIssueId: $c}) { subIssue { number } } }' \
  -F p="$PARENT" -F c="$CHILD"
```

紐付けたら親 Issue のチェックリストにも `#68` を書き足す(`ghp issue edit 67 --body-file`)。

## ブランチ

`main` から切る。`<type>/<issue番号>-<英語の短い説明>` を基本にし、Issue が無い文書変更は番号を省く。

```text
feat/68-frontend-shared-foundation
test/59-receipt-flow-integration
docs/frontend-skill
fix/pull-request-ci-triggers
refactor/analyze-receipt-feature-package
chore/backend-coding-rules
```

`type` は commit と同じ(`feat` / `fix` / `refactor` / `test` / `docs` / `chore`)。

## commit

Conventional Commits の形で、要約は日本語にする。scope はリポジトリの領域(`front` / `backend` / `infra` / `skills` / `docs`)。

```text
<type>(<scope>): <何をどうしたか(体言止めか「〜する」)>

<なぜ・どの方針に基づくかを 1〜2 文>

- 変更点を層やファイル群ごとに箇条書き
- 振る舞いが変わる点、削除した点も書く

Refs #68
```

- 1 commit は 1 つの Issue に対応させる。別 Issue の変更を混ぜない。
- Issue を閉じるのは PR に任せ、commit では `Refs #N` にとどめる。
- AI ツールの署名行(`Co-Authored-By` など)は各ツールの規約に従い、末尾に置く。
- commit 前に対象領域の検証コマンドを通す(下記「検証」)。

例:

```text
feat(front): デザイントークンとグローバルスタイルを整える

design_guidelines.md のトークンを app/styles/globals.css に実装し、画面の直書きの色・余白をトークン参照に置き換える。

- app/styles/globals.css: 色・余白・角丸・影・画面幅のトークン、box-sizing reset、system font stack、focus indicator、.page-shell、.amount、prefers-reduced-motion
- app/styles/screens.css: 旧 index.css の画面スタイルを移し、値をトークンに揃える。ブレークポイントを 600px / 960px に合わせる
- 解析依頼の状態バッジを色の役割(neutral / info / primary / warning / danger)で表現し、期限切れを warning、解析中を info にする

Refs #70
```

## 検証

PR を作る前に、変更した領域の標準コマンドをすべて実行し、結果を PR の `## 検証` に実際の値で書く。

| 領域 | コマンド |
| --- | --- |
| `front/` | `pnpm lint` / `pnpm test` / `pnpm build` / `git diff --exit-code -- package.json pnpm-lock.yaml`(依存追加時は差分が意図どおりかと `technology_stack.md` の更新を確認) |
| `backend/` | `go test -race ./...` / `go vet ./...` / `go mod tidy` 後に `go.mod` `go.sum` の差分なし |
| 文書のみ | `git diff --check` |

失敗したまま PR を作らない。既知の警告が残る場合は件数と理由を書く。

## Pull Request

- base は `main`。タイトルは commit と同じ形(`feat(front): 〜`)。複数 commit なら全体を表す 1 行にする。
- 1 PR で複数 Issue を閉じる場合は `Closes #68, closes #70(親: #67)` と全部書く。親 Issue は閉じない。
- 本文は `--body-file -` の heredoc で渡す。作成後に本文を書き換える場合、`ghp pr edit` が Projects (classic) のエラーで失敗することがあるので REST を使う。

```sh
ghp api -X PATCH repos/OWNER/REPO/pulls/69 --input body.json   # {"title": "...", "body": "..."}
```

### 本文の構成

```markdown
## 概要

何を、どの文書・方針に基づいて行うか(2〜3 文)。

Closes #68(親: #67)

## 変更内容

### <層や領域ごとの小見出し>(shared / features / app、backend / infra / docs など)

- ファイルと役割を 1 行ずつ。判断した理由は括弧で添える

## 利用者から見える変更   ← ある場合だけ

- 画面の文言、URL、表示の変化

## 残課題(別 Issue)   ← 意図的に残したものがある場合だけ

- 何を、なぜ今やらないか(導入タイミング、別 Issue の番号)

## 検証

`front/` で実行。

- `pnpm lint`: 警告 0
- `pnpm test`: 9 files / 54 tests pass
- `pnpm build`: 成功
- `package.json` / `pnpm-lock.yaml`: `react-router` 追加分のみ(`technology_stack.md` を同時更新)
```

末尾に AI ツールの署名行がある場合は各ツールの規約に従う。

### 作成後

- `ghp pr checks <番号>` で CI(`build`)が通ることを確認する。
- 前提となる Issue / 親 Issue のチェックリストが最新かを確認する。

## マージ

- GitHub 上の「Create a merge commit」で行う(squash / rebase は使わない)。マージコミットは GitHub 既定の `Merge pull request #69 from OWNER/branch` に PR タイトルが本文として付く形のままにし、手で書き換えない。
- Issue は PR の `Closes` で自動的に閉じる。閉じた Issue に補足が必要なら、閉じる前にコメントで「何をどこまでやったか」「残りはどの Issue か」を書く。
- マージ後はローカルの `main` を更新し、作業ブランチを削除する。

## deploy ブランチへの反映

`docs/ci_cd/codepipeline-design.md` のとおり、stage ごとの deploy ブランチ(`deploy/dev` / `deploy/stg` / `deploy/prd`)を進めることで配布する。

- 通常は `main` を `deploy/dev` に merge する。
- マージ前に dev で動作確認したい場合は、作業ブランチを `deploy/dev` に merge して push する。`deploy/dev` に `main` に無い独自コミットが無いことを先に確認する。

```sh
git fetch origin
git log --oneline origin/main..origin/deploy/dev   # 空であることを確認
git checkout -B deploy/dev origin/deploy/dev
git merge --no-edit <作業ブランチ または main>
git push origin deploy/dev
git checkout <作業ブランチ>
```

依頼されていない stage(`deploy/stg` / `deploy/prd`)へは push しない。

## 完了報告

ユーザーへの報告は次を表か箇条書きで返す。

- 作成した Issue / PR の番号、タイトル、URL
- `deploy/*` に push した場合はブランチと commit
- 検証結果(実際の数値)
- 残課題と、それがどの Issue に紐づくか
- コミットしていない・push していないものがあればその旨
