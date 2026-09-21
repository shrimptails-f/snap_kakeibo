# アップロード履歴画面

## 目的

月ごとの請求書・レシート画像単位のアップロード履歴を確認する。

解析に失敗した画像や、まだ解析中の画像を追えることを優先する。

---

## 画面設計

指定月のアップロード履歴を画像単位で一覧表示する。

```text
アップロード日時

ファイル名

解析ステータス

店舗名

請求金額
```

`SUCCEEDED` の履歴行を押すと、請求詳細・編集画面を開く。

詳細表示は別画面またはモーダルのどちらでも実装可能とする。

---

## ステータス表示

バックエンドの `status` に加えて、画面側で経過時間を判定して表示を切り替える。バックエンドは期限切れや停滞で状態を変更しない。

| 表示 | 条件 | 操作 |
| --- | --- | --- |
| アップロード待ち | `status = UPLOADING` かつ `now <= expires_at` | なし |
| 期限切れ | `status = UPLOADING` かつ `now > expires_at` | 再アップロードを促す |
| 解析中 | `status = ANALYZING` かつ `now - updated_at <= 30分` | なし |
| 停滞 | `status = ANALYZING` かつ `now - updated_at > 30分` | 再実行ボタン |
| 完了 | `status = SUCCEEDED` | 請求詳細・編集画面へ |
| データなし | `status = NO_DATA` | 再実行ボタン |
| 失敗 | `status = FAILED` | `error_code` に応じたメッセージ + 再実行ボタン |

### error_codeごとの表示

| error_code | メッセージ | 再実行 |
| --- | --- | --- |
| `ANALYSIS_FAILED` | 画像を解析できませんでした | 可 |
| `INVALID_DATE` | 購入日を正しく読み取れませんでした | 可 |
| `INVALID_AMOUNT` | 金額を正しく読み取れませんでした | 可 |
| `NO_TOTAL_AMOUNT` | 合計金額を読み取れませんでした | 可 |
| `NO_DATE` | 購入日を読み取れませんでした | 可 |
| `TOO_MANY_DETAILS` | 商品明細が50件を超えています | 不可(再アップロードを促す) |
| `INTERNAL` | システムエラーが発生しました | 可 |

`NO_DATA` は解析完了だが登録対象の明細が0件だった状態。請求と月次集計には反映しない。

一時的な失敗はバックエンドが自動で最大3回まで再実行する。それでも失敗したものや恒久的な失敗には手動再実行ボタンを表示する。

`TOO_MANY_DETAILS` は初期構成の仕様上扱えないため、再実行ボタンは出さない。

---

## API設計

### GET /months/{yyyy-MM}/uploads

指定月のアップロード履歴を作成日時の降順で取得する。`{yyyy-MM}` が `YYYY-MM` の形式でなければ `400`。

Response:

```json
{
  "items": [
    {
      "upload_id": "01JUPLOADXXX",
      "billing_id": "01JBILLXXX",
      "status": "SUCCEEDED",
      "file_name": "receipt.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "created_at": "2026-09-15T12:00:00Z",
      "updated_at": "2026-09-15T12:01:00Z"
    },
    {
      "upload_id": "01JUPLOADYYY",
      "status": "NO_DATA",
      "file_name": "receipt-2.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    },
    {
      "upload_id": "01JUPLOADZZZ",
      "status": "FAILED",
      "file_name": "receipt-3.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "error_code": "NO_TOTAL_AMOUNT",
      "error_message": "合計金額を取得できませんでした",
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    }
  ]
}
```

| 項目 | 備考 |
| --- | --- |
| `billing_id` | `SUCCEEDED` のときだけ。請求詳細・編集画面への遷移に使う |
| `error_code` / `error_message` | `FAILED` のときだけ |
| `items` | 該当が無ければ `[]` |

値が無い項目は `null` ではなく省略する。ページネーションは持たない(1 回の Query の範囲を返す)。

#### 画面設計に対して未対応の項目

以下は画面設計にあるが API が返していない。対応するときは backend / frontend / 本書を合わせて変える。

| 項目 | 用途 | 未対応の理由 |
| --- | --- | --- |
| `expires_at` | 「期限切れ」表示(`UPLOADING` かつ `now > expires_at`) | `upload_histories` にはあるが返していない |
| `attempt` / `failed_at` | 再実行回数・失敗日時の表示 | 同上 |
| `store_name` / `final_amount` | 一覧の「店舗名」「請求金額」列 | `billings` にしか無く、一覧で請求を結合していない |

### POST /uploads/{upload_id}/retry

`FAILED`、`NO_DATA`、または停滞した `ANALYZING` の画像に対して、解析を最初からやり直す。

Response:

```json
{
  "upload_id": "01JUPLOADYYY",
  "status": "ANALYZING",
  "attempt": 3
}
```

`status` が `FAILED` / `NO_DATA` / `ANALYZING` 以外の場合は `409 Conflict` を返す。
