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
| routing | React Router | 画面と URL の分離、認証ガード(`src/app/router`) |
| server state | TanStack Query | API 由来のデータの取得、cache、再取得、mutation。初回 loading は Suspense で扱う |
| form | React Hook Form | 入力状態、送信中、validation lifecycle(`features/*/components` のフォーム) |
| validation | Zod | フォーム入力と API レスポンスの境界検証(`*.schema.ts`、`shared/api/parseResponse`) |
| styling | CSS Modules | component ごとの `*.module.css`。トークン・リセット・全画面共通の class は `app/styles/globals.css` |

Node.js と pnpm の実行環境はリポジトリの Dev Container に合わせる。

### レシート画像の編集

トリミングは標準の Canvas / HTMLImageElement / Pointer Events / HTMLDialogElement を使う。EXIFの向き補正済みの画像から元解像度で切り抜く。自動範囲検出は長辺480px以下の画像を専用Web Workerへ渡し、終了・キャンセル時にWorkerを破棄する。

追加依存・AIモデルは導入しない。OpenCV.jsは、矩形範囲の提案に対して配信・初期化の負担が増えるため現時点では採用しない。精度・配信サイズ・処理時間は [トリミング評価](../../docs/receipt-cropping-evaluation.md) に記録する。

## 3. 採用方針

現時点で「採用方針」のままの技術はない。新しい技術は、まずここに用途と導入条件を書いてから実装する。

## 4. 保留

### スタイリング方式

CSS Modules を採用した(§2)。Tailwind CSS や CSS-in-JS は、次のいずれかが具体化するまで再検討しない。

- design token を CSS custom properties だけで扱えなくなる(テーマ切替など)
- 動的な style が増え、class の組み合わせで表現しきれなくなる

### UI component library

全面的な UI library は現時点で導入しない。Dialog、Popover、Select などアクセシビリティ実装が難しい部品が必要になった場合は、headless component library を優先して比較する。

### chart library

ダッシュボードと月別支出画面は、追加依存なしの CSS で実装した。金額・カテゴリ・月への導線を通常の HTML でも提供し、グラフはその比較を補助する表現に限定しているためである。

今後、複雑な可視化が必要になり標準 API と CSS では保守しにくくなった場合は、次を必須条件として選定する。

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
- server state の cache と再取得は TanStack Query に集約し、feature の `hooks` に `useXxx` として置く。フォーム入力やダイアログ開閉などの UI 状態には使わない
- 取得は `useSuspenseQuery` を基本とし、初回 loading はレイアウト(`AppLayout` / `GuestLayout`)の `Suspense` が受ける。一部だけ先に出したいパネルは画面側で `Suspense` と `ErrorBoundary` を追加する
- 想定外の失敗は画面ルートの `errorElement` で受ける。ガードより内側に置き、セッション切れは `AuthGuard` がログイン画面へ送る
- 認証セッションは cache ではなくアプリ状態として `AuthSessionProvider` が持ち、Query には載せない

## 6. テスト戦略

| レベル | 技術 | 対象 |
| --- | --- | --- |
| 単体 | Vitest | 判定、変換、format、hook |
| component | React Testing Library | 表示、入力、利用者操作、状態遷移 |
| API mock | 保留 | feature をまたぐ通信シナリオ |
| E2E | 保留 | 認証、アップロード、解析結果確認の主要導線 |

API mock と E2E のライブラリは、複数画面の主要導線が実装された時点で選定する。E2E は少数の重要シナリオに限定し、component test と重複させない。

`pnpm dev` では、API 接続先が未設定のときだけ Vite の開発用 middleware が閲覧用サンプル API を返す。これは手元で画面を確認するためのデータであり、上表の自動テスト用 API mock とは別に扱う。

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
