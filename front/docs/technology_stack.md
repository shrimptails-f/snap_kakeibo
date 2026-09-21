# 技術スタック

## 1. ステータスの定義

この文書では、技術の状態を次の3つに分ける。

| 状態 | 意味 |
| --- | --- |
| 採用済み | 現在の `package.json` と実装で使用している |
| 採用方針 | 導入時の標準として採用するが、まだ実装されていない場合がある |
| 保留 | 必要性が生じた時点で比較・決定する |

バージョンの正は `package.json` と `pnpm-lock.yaml` とし、この文書には固定バージョンを重複記載しない。

## 2. 採用済み

| 分類 | 技術 | 用途 |
| --- | --- | --- |
| UI | React | component による画面構築 |
| 言語 | TypeScript | 静的型付け |
| build | Vite | 開発サーバーと本番 build |
| package manager | pnpm | 依存管理と script 実行 |
| lint | Oxlint | 静的解析 |
| test | Vitest | test runner |
| UI test | React Testing Library | 利用者視点の component test |
| DOM test | jsdom | test 用ブラウザ環境 |
| HTTP | Fetch API | バックエンドおよび Presigned URL への通信 |
| styling | CSS | global style と responsive design |

Node.js と pnpm の実行環境はリポジトリの Dev Container に合わせる。

## 3. 採用方針

### React Router

画面と URL を分離し、戻る・再読み込み・直接アクセスを成立させるために導入する。ルート定義は `src/app/router` に集約する。

導入タイミングは、現在の単一画面から最初の画面分割を行う変更とする。

### TanStack Query

API 由来のサーバー状態、cache、再取得、mutation を管理するために導入する。解析依頼の polling、認証後の取得、支出詳細の取得を component 内の `useEffect` から分離する。

導入後も、フォーム入力やダイアログ開閉などの UI 状態には使用しない。

### React Hook Form + Zod

ログイン以外の編集フォームが増える段階で導入する。

- React Hook Form: 入力状態と validation lifecycle
- Zod: 入力値と外部レスポンスの runtime validation

小さなフォーム一つだけの段階で抽象化を増やすのではなく、支出編集など複数項目のフォーム実装開始を導入目安とする。

## 4. 保留

### スタイリング方式

現時点では CSS を継続する。CSS Modules、Tailwind CSS、CSS-in-JS の追加導入は決定していない。

選定時は次を比較する。

- style の適用範囲を feature 内に閉じられるか
- design token と responsive design を一貫して扱えるか
- class の可読性と component test を損なわないか
- build、保守、依存更新の負担

方針決定までは、具体的な class 名と CSS custom properties で衝突を防ぐ。

### UI component library

全面的な UI library は現時点で導入しない。Dialog、Popover、Select などアクセシビリティ実装が難しい部品が必要になった場合は、headless component library を優先して比較する。

### chart library

ダッシュボードと月別支出画面の実装前に選定する。次を必須条件とする。

- React で安定して利用できる
- responsive 表示に対応する
- keyboard と screen reader 向けの代替表現を用意できる
- bundle size が用途に見合う
- 積み上げ棒グラフ、円または同等の構成比表示、横棒グラフを扱える

円グラフを使うこと自体も固定せず、比較しやすさとアクセシビリティから棒グラフ等への変更を許容する。

### global client state library

Redux、Zustand 等は導入しない。URL、サーバー状態、フォーム状態、component state で表現できない共有状態が具体化した場合にのみ再検討する。

## 5. API 方針

- 同一 origin の `/api/*` または `VITE_API_BASE_URL` を Fetch API で呼び出す
- 通常 API は共通 HTTP client を経由する
- Presigned URL へのアップロードは認証付き API client と分離する
- DTO はバックエンドの JSON 契約に合わせる
- OpenAPI 等による型生成は、API 仕様の機械可読な正本を導入する時点で検討する
- server state の cache と再取得は TanStack Query 導入後に集約する

## 6. テスト戦略

| レベル | 技術 | 対象 |
| --- | --- | --- |
| 単体 | Vitest | 判定、変換、format、hook |
| component | React Testing Library | 表示、入力、利用者操作、状態遷移 |
| API mock | 保留 | feature をまたぐ通信シナリオ |
| E2E | 保留 | 認証、アップロード、解析結果確認の主要導線 |

API mock と E2E のライブラリは、複数画面の主要導線が実装された時点で選定する。E2E は少数の重要シナリオに限定し、component test と重複させない。

## 7. 品質チェック

現在の標準コマンドは次のとおり。

```bash
pnpm dev
pnpm lint
pnpm test
pnpm build
pnpm preview
```

CI では少なくとも lint、test、build を実行する。formatter、アクセシビリティ自動検査、bundle size 検査は未導入であり、必要性と運用方法を決めてから追加する。

## 8. 依存追加の判断基準

依存を追加する前に次を確認する。

1. 現在の標準 API と既存依存では解決しにくいか
2. アプリ内の複数箇所、または重要な複雑性を解消するか
3. maintenance 状況、license、security、bundle size は許容できるか
4. Dev Container、CI、production build で動作するか
5. この文書とアーキテクチャへ役割を説明できるか

導入した技術は `package.json` だけでなく、本書の状態も更新する。
