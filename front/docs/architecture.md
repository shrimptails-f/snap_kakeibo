# フロントエンドアーキテクチャ

## 1. 目的

画面、業務機能、通信処理を分離し、機能追加時に変更範囲を予測できる構造にする。初期段階で過剰に共通化せず、コードの所有場所と依存方向を明確にする。

## 2. 基本構成

`app / features / shared` の3層を採用する。

```text
src/
├── app/
│   ├── App.tsx
│   ├── layouts/
│   ├── providers/
│   ├── router/
│   └── styles/
├── features/
│   ├── auth/
│   ├── dashboard/
│   ├── expenses/
│   └── receipt-analysis/
├── shared/
│   ├── api/
│   ├── config/
│   ├── lib/
│   ├── types/
│   └── ui/
│       └── Button/
├── test/
└── main.tsx
```

必要な場所だけに下位ディレクトリを作る。空のディレクトリを将来用に一括作成しない。

### `app`

アプリケーション全体の組み立てを担当する。

- Provider の合成
- ルート定義と認証ガード
- ページ共通レイアウト
- グローバルスタイル
- feature の画面を URL に割り当てる処理

業務ロジックや個別 API のレスポンス型は置かない。

### `features`

利用者から見た業務機能ごとにコードをまとめる。snap_kakeibo では次を最初の境界とする。

| feature | 責務 |
| --- | --- |
| `auth` | ログイン、ログアウト、セッション復元 |
| `dashboard` | 月次集計の俯瞰 |
| `expenses` | 月別支出、支出詳細、支出編集 |
| `receipt-analysis` | レシート画像のアップロード、解析依頼一覧、再解析 |

feature 内は必要に応じて次のように分ける。

```text
features/expenses/
├── api/
├── components/
├── hooks/
├── lib/
├── screens/
├── types/
└── index.ts
```

- `screens`: ルートから呼ばれる画面の入口
- `components`: feature 内だけで使う表示部品
- `hooks`: ユースケースと UI をつなぐ処理
- `api`: エンドポイント単位の通信関数と通信 DTO
- `types`: feature 内で共有する型
- `lib`: 副作用を持たない変換・判定処理

### `shared`

業務機能を知らない共通基盤を置く。

- HTTP クライアントと共通エラー型
- Button、TextField、Dialog などの汎用 UI
- 日付・金額の汎用フォーマット
- 環境変数の読み取り
- テストヘルパー

「将来使いそう」という理由では移動しない。2つ以上の feature で実際に必要になり、意味と振る舞いが同じ場合に共通化する。

## 3. 依存方向

```text
main.tsx
   ↓
  app ─────→ features
   │            │
   └────────────┴──→ shared
```

- `app` は `features` と `shared` を参照できる
- `features` は `shared` を参照できる
- `shared` は `app` と `features` を参照しない
- feature 間の内部ファイルを直接参照しない
- feature 外へ公開するものは、その feature の `index.ts` に限定する

feature 間で連携が必要な場合は、URL、共通化された型、または `app` での合成を優先する。循環依存を解決する目的だけで `shared` に業務ロジックを移さない。

## 4. データフロー

```text
利用者操作
  ↓
screen / component
  ↓
feature hook
  ↓
feature api
  ↓
shared HTTP client
  ↓
バックエンド API
```

通信 DTO は API の snake_case に合わせる。画面で別の形が必要な場合は feature の境界で ViewModel へ変換し、API 都合を表示コンポーネント全体へ広げない。

状態の所有者は次のように決める。

| 状態 | 所有場所 |
| --- | --- |
| URL、選択中の月、支出 ID | Router のパスまたはクエリ |
| API から取得したデータ | サーバー状態管理層 |
| 入力途中の値 | フォームまたは画面ローカル state |
| ダイアログ開閉など一時的な表示 | 利用箇所に最も近い component |
| 認証セッション | `auth` とアプリ全体の Provider |

サーバーから取得した値を、理由なく別のローカル state に複製しない。

## 5. 認証と API

- API 呼び出しは component に直接記述せず、feature の `api` を経由する
- API のベース URL は `shared/config` で一度だけ解決する
- Cookie を利用する refresh と Bearer access token の責務を共通 HTTP クライアントへ集約する
- `401` 時の refresh と再送は同時実行を考慮し、各 component で個別実装しない
- 画面に表示するエラー文言へ HTTP ステータス文字列をそのまま露出しない
- Presigned URL への PUT は通常の API と送信先・認証方式が異なるため、専用関数として扱う

業務用語と API 契約は [ユビキタス言語](../../docs/ddd/ubiquitous-language.md) と [画面仕様](../../docs/screens/) に合わせる。

## 6. ルーティング方針

ルート定義は `app/router` に集約する。想定する URL は画面仕様を基準にし、少なくとも次の単位を独立して開ける構造にする。

```text
/login
/
/months/:yearMonth
/analysis-requests
/expenses/:expenseId
```

- 一覧の絞り込みや選択月は、再読み込み・共有が有用なら URL に持たせる
- screen はルートのパラメータを受け取り、feature の UI を組み立てる薄い入口にする
- ナビゲーションを伴う要素には、可能な限りリンクを使用する

新しい画面は `app/router/routes.tsx` にルートとして追加し、既存画面の条件分岐だけで増やさない。

## 7. 移行方針

初期の検証実装(認証、アップロード、ポーリング、一覧、詳細表示を一つに持つ `App.tsx`)を、一括リライトではなく次の順で分離する。

1. 純粋な型・表示変換・HTTP クライアントを分離する(完了: `shared/api`、`shared/auth`、`shared/lib`)
2. 認証と解析依頼を feature に分離する(完了: 認証は `features/auth`、アップロードは `/upload`、解析履歴は `/analysis-requests`、支出詳細・編集は `features/expenses` の `/expenses/:expenseId`。解析履歴の月・状態絞り込みとページネーションは後続実装)
3. Router とページ共通レイアウトを導入する(完了: `app/router`、`app/layouts`、`/upload`、`/analysis-requests`、`/expenses/:expenseId`。`/months/:yearMonth` は画面の実装時に追加する)
4. API データ取得をサーバー状態管理層へ移す(完了: TanStack Query。取得は feature の `hooks`、初回 loading はレイアウトの `Suspense`)
5. 共通化の実績ができた UI だけを `shared/ui` へ移す(完了: `Spinner`、`ErrorBoundary`、`Button`。TextField や状態バッジなどは画面仕様の確定後に判断する)

各段階で既存テストを保ち、利用者から見た振る舞いを変える場合は画面仕様も更新する。
