---
name: snap-kakeibo-frontend
description: snap_kakeibo リポジトリの React + TypeScript フロントエンド（front/）を実装・変更・レビューする。app / features / shared の構成、画面、hook、API 関数、共通 UI、スタイル、フロントエンドテストが対象。backend や infra だけの変更には使用しない。
---

# snap_kakeibo フロントエンド実装

## 着手前

1. `git rev-parse --show-toplevel` でリポジトリルートを取得する。
2. リポジトリルートの `AGENTS.md` を読む。
3. 次の文書を省略せず全文読む。
   - `front/docs/README.md`
   - `front/docs/architecture.md`
   - `front/docs/coding_rules.md`
   - `front/docs/design_guidelines.md`
   - `front/docs/technology_stack.md`
4. 変更対象の画面に対応する `docs/screens/<screen>/README.md` と `docs/ddd/ubiquitous-language.md` を読み、画面要件と業務用語を確認する。
5. 変更対象に近い既存の component、hook、API 関数、テストを確認し、リポジトリ内の実例に揃える。

判断が食い違う場合の優先順位は `front/docs/README.md` に従う（ユビキタス言語と画面仕様 → `front/docs` の設計・規約 → 現在の実装）。

上記文書が存在しない場合や、対象が snap_kakeibo ではない場合は、このスキル固有の構成を当てはめず、通常のリポジトリ調査へ切り替える。

## 実装

- レイヤーの責務、依存方向、命名、TypeScript / React の書き方、スタイリング、アクセシビリティ、テスト方針は上記文書を正本とし、このスキル内に複製しない。
- 採用済み・採用方針・保留の技術区分は `front/docs/technology_stack.md` を正とする。保留の技術を独断で導入せず、採用方針の技術は文書に書かれた導入タイミングに達しているか確認してから導入する。
- 依存を追加する場合は `front/docs/technology_stack.md` の判断基準を満たすことを確認し、`package.json` と同じ変更で同文書の状態を更新する。
- `front/src/features/receipt-analysis/screens/ReceiptIntakePage.tsx` は移行途中の検証実装である。新しい処理をこの画面へ積み増さず、`front/docs/architecture.md` の移行方針に沿って変更範囲を feature / shared へ分離する。
- 変更対象と同じ feature の `screens`、`components`、`hooks`、`api`、`types`、`lib`、`index.ts` と、それを組み立てる `app` を横断して影響を確認する。
- API パス、DTO、認証方式、環境変数（`VITE_*`）を変更する場合は、front に加えて backend、infra、tests、docs を検索し、契約の整合を保つ。
- 利用者から見える振る舞いや画面項目を変える場合は、`docs/screens` の画面仕様を同じ変更で更新する。画面仕様にない操作や項目をデザインだけで追加しない。
- ユーザーの依頼範囲を維持する。触れたコードで見つけた明白かつ軽微な問題は修正してよいが、無関係な全面整形や大規模リファクタリングへ広げない。
- 文書と既存コードに不一致がある場合、暗黙にリポジトリ全体を移行しない。変更範囲で安全な既存パターンを選び、不一致をユーザーへ報告する。

## 検証

変更内容に応じた狭いテストを先に実行し、完了前に `front/` で次を実行する。

```sh
pnpm lint
pnpm test
pnpm build
git diff --exit-code -- package.json pnpm-lock.yaml
```

依存の追加・更新を意図するタスクでは、最後の差分検査を失敗条件にせず、その差分が必要かと `front/docs/technology_stack.md` が更新されているかを確認する。

テストは利用者から見える振る舞いを `getByRole` を第一候補に検証し、API 成功・空・失敗・再試行など重要な画面状態を網羅する。時刻依存の判定は現在時刻を固定し、テスト間で共有する mock はリセットして実行順へ依存させない。
