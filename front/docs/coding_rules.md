# コーディング規約

## 1. 基本方針

- React と TypeScript を前提とし、型エラーを残さない
- 業務用語は [ユビキタス言語](../../docs/ddd/ubiquitous-language.md) に合わせる
- コードは使用箇所に近い場所へ置き、実績のない共通化をしない
- component は表示と操作の組み立てに集中させ、通信や複雑な変換を分離する
- 既存コードの変更時は、関連する範囲を本規約へ近づける。無関係な全面整形は行わない

## 2. ファイルと名前

| 対象 | 規則 | 例 |
| --- | --- | --- |
| React component / screen | `PascalCase.tsx` | `ExpenseDetailPage.tsx` |
| hook | `use` + `PascalCase.ts` | `useExpense.ts` |
| API | `kebab-case.api.ts` | `analysis-requests.api.ts` |
| 型 | `kebab-case.types.ts` | `expense.types.ts` |
| schema | `kebab-case.schema.ts` | `login.schema.ts` |
| 汎用関数 | `camelCase.ts` | `formatYen.ts` |
| テスト | 対象名 + `.test.ts(x)` | `ExpenseCard.test.tsx` |

- component と型は役割が伝わる具体名にする
- `data`、`item`、`handleClick` のような名前は、狭いスコープ以外では対象と操作を明示する
- ID は業務語に合わせて `expenseId`、`analysisRequestId` とし、単独の `id` を避ける
- boolean は `is`、`has`、`can`、`should` で始める
- イベント Props は `onSubmit`、内部ハンドラは `handleSubmit` の形を基本とする

## 3. TypeScript

- `any` は使用しない。不明な外部値は `unknown` として検証する
- object の形には原則 `type` を使い、宣言マージが必要な場合だけ `interface` を使う
- component の Props は component の直前に `type Props` として定義する
- export する関数と API 関数は、返り値の型を明示する
- 型アサーションで API レスポンスの安全性を保証したことにしない。境界で zod schema(`*.schema.ts`)により検証し、型は `z.infer` で schema から導く
- optional と `null` を無目的に混在させず、API 契約に合わせる
- 金額は円の整数、日付は用途に応じた文字列として扱い、暗黙に `Date` へ変えない

```tsx
type Props = {
  amount: number
  isEdited?: boolean
}

export function RecordedAmount({ amount, isEdited = false }: Props) {
  return (
    <p>
      {formatYen(amount)}
      {isEdited && <span>手動編集済み</span>}
    </p>
  )
}
```

## 4. React

- component は関数宣言を基本とする
- 1ファイル1 component を基本とするが、外部で再利用しない小さな表示要素は同居してよい
- 派生値は render 中に計算し、同期目的だけの `useEffect` を避ける
- `useEffect` は外部システムとの同期に限定する
- state は必要な最小単位で、利用箇所に最も近い component が所有する
- 配列の `key` には永続 ID を使い、順序が変わる一覧で index を使わない
- `useMemo` と `useCallback` は、参照の安定性または計測済みの性能上の理由がある場合に使う
- 非同期処理では loading、empty、error、success の各状態を設計する

screen はルートと feature の接続に留める。通信、状態判定、巨大な JSX が一つの screen に集まり始めたら hook や component へ分割する。

## 5. import と公開 API

- import は外部パッケージ、絶対パス、相対パスの順にまとめる
- feature 内部は相対 import、feature 外からは公開 `index.ts` を使う
- `shared/index.ts` のような全体 barrel は作らない
- `shared/ui` は部品ごとにディレクトリを切り(`shared/ui/Button/`)、component・`*.module.css`・test を同じディレクトリに置く。外からは `@/shared/ui/Button` のようにディレクトリの `index.ts` を経由する
- 型だけの import は `import type` を使う
- 深い相対パスを避けるため、`@/` を `src/` に割り当てる
- 循環依存を避けるため、feature 内部から自身の `index.ts` を経由しない

## 6. API とエラー

- component から直接 `fetch` しない
- API パス、メソッド、Request/Response DTO は `features/*/api` に置く
- 認証ヘッダー、Cookie、JSON の読み書き、共通エラー変換は `shared/api` に置く
- `Response.ok` だけでなく、想定するレスポンス形式を境界で確認する。API 関数は `shared/api/parseResponse` にレスポンスの schema を渡し、手書きの型ガードを増やさない
- フォームは React Hook Form と zod resolver を使い、入力の schema は `features/*/types/*.schema.ts` に置く。サーバー由来の失敗は `setError('root.server')` でフォーム全体のエラーにする
- `catch` した値は `unknown` として扱う
- 利用者向け文言とログ・調査用情報を分ける
- エラーを握りつぶす場合は、無視して安全な理由をコメントまたは関数名で明確にする

API のフィールド名は境界では snake_case のまま扱ってよい。UI 用モデルに変換する場合は、feature 内で変換方法を統一する。

## 7. スタイリング

- 色、余白、角丸、影、画面幅は [デザインガイドライン](./design_guidelines.md) の CSS custom properties を使う
- 業務上の状態を色だけで表現しない
- component の style は同じディレクトリの `<Component>.module.css` に置き、`import styles from './X.module.css'` で参照する。class 名は camelCase(`.fieldError`)にし、BEM の接頭辞は付けない
- グローバル CSS は `app/styles/globals.css` だけとし、トークン、リセット、`.page-shell` / `.amount` / `.muted` のような全画面共通の class に限定する。feature 固有の class を追加しない
- 状態による見た目の切り替えは、`aria-invalid` などの属性セレクタか、`.status` + `.statusDanger` のような追加 class で表す
- 別 component の module を import するのは、同じ見た目を意図的に共有する場合(`GuestLayout` が `AppHeader.module.css` を使うなど)に限る。2 つ以上の feature で同じ部品が必要になったら `shared/ui` へ移す
- `shared/ui` の部品に `className` を渡すのは幅や配置など置き場所の都合に限り、色や形は部品の props(`Button` の `variant` など)で選ぶ
- inline style は動的な数値など、CSS で表現しにくい場合に限定する
- `!important` は原則使用しない

## 8. アクセシビリティ

- 操作には意味に合う HTML 要素を使う。遷移は `a`、実行は `button` を基本とする
- すべての入力に可視ラベルを関連付ける。placeholder をラベル代わりにしない
- キーボードだけですべての操作を完了できるようにする
- focus indicator を消さない
- アイコンだけのボタンには accessible name を付ける
- 非同期の完了・失敗通知には適切な `aria-live` を使う
- 見出しレベルを画面構造に合わせる

## 9. テスト

- Vitest と React Testing Library を使う
- 実装内部ではなく、利用者から見える振る舞いを検証する
- 要素取得は `getByRole` を第一候補とし、ラベル、表示テキストの順で検討する
- `data-testid` は意味のある取得方法がない場合だけ使う
- 利用者操作には `userEvent` を使う
- API 成功、空、失敗、再試行など重要な画面状態を検証する
- 時刻依存の判定は現在時刻を固定して検証する
- テスト間で共有する mock はリセットし、実行順へ依存させない

最低限、変更前に次を実行する。

```bash
pnpm lint
pnpm test
pnpm build
```

## 10. 禁止事項

- component からの直接 `fetch`
- `any` による型エラーの回避
- API 由来のサーバー状態を複数箇所へコピーして同期する実装
- feature 内だけの処理を先回りして `shared` へ置くこと
- 色だけに依存した状態表現
- ラベルのない入力
- 役割の異なる処理を `App.tsx` へ追加し続けること
