---
name: snap-kakeibo-backend
description: snap_kakeibo リポジトリの Go バックエンドを実装・変更・レビューする。Lambda、feature package、DI、DynamoDB repository、共通 library、バックエンドテストが対象。frontend や infra だけの変更には使用しない。
---

# snap_kakeibo バックエンド実装

## 着手前

1. `git rev-parse --show-toplevel` でリポジトリルートを取得する。
2. リポジトリルートの `AGENTS.md` を読む。
3. 次の文書を省略せず全文読む。
   - `backend/docs/coding_rules.md`
   - `backend/docs/directory-structure.md`
4. 変更対象に近い既存実装、テスト、DI 定義を確認し、リポジトリ内の実例に揃える。

上記文書が存在しない場合や、対象が snap_kakeibo ではない場合は、このスキル固有の構成を当てはめず、通常のリポジトリ調査へ切り替える。

## 実装

- レイヤーの責務、依存方向、命名、テスト方針は上記2文書を正本とし、このスキル内に複製しない。
- logger と Lambda instrumentation は、現在の `backend/internal/library/logger` と `backend/internal/library/lambdawrap` の実装を正とする。
- 変更対象と同じ feature の application、domain、infrastructure、library、DI、`cmd/<lambda>` を横断して影響を確認する。
- Lambda 名、API パス、設定値、外部公開 contract を変更する場合は、backend に加えて infra、front、tests、docs を検索する。
- ユーザーの依頼範囲を維持する。触れたコードで見つけた明白かつ軽微な問題は修正してよいが、無関係な大規模リファクタリングへ広げない。
- 文書と既存コードに不一致がある場合、暗黙にリポジトリ全体を移行しない。変更範囲で安全な既存パターンを選び、不一致をユーザーへ報告する。

## 検証

変更内容に応じた狭いテストを先に実行し、完了前に `backend/` で次を実行する。

```sh
gofmt -w <変更した Go ファイル>
go test -race ./...
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
```

`go mod tidy` が依存ファイルを意図どおり変更するタスクでは、最後の差分検査を失敗条件にせず、その差分が必要かを確認する。

テストを並列化する場合は、環境やファイルへの依存を `oswrappertest.Mock` などへ差し替える。Floci の AWS リソースにはテストごとのランダム ID を付け、`t.Cleanup` で自分のリソースだけを削除する。process-global state や他テストの cleanup 順序へ依存しない状態にしてから `t.Parallel()` を追加する。
