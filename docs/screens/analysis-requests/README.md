# 解析依頼一覧画面

## 目的

月ごとのレシート画像単位の解析依頼(受付から解析の終端まで)を確認する。

解析に失敗した画像や、まだ解析中の画像を追えることを優先する。

---

## 画面設計

指定月の解析依頼を画像単位で一覧表示する。

```text
アップロード日時

ファイル名

解析ステータス

店舗名

計上額
```

`SUCCEEDED` の行を押すと、支出詳細・編集画面を開く。

詳細表示は別画面またはモーダルのどちらでも実装可能とする。

---

## ステータス表示

バックエンドの `status` に加えて、画面側で経過時間を判定して表示を切り替える。バックエンドは期限切れや停滞で状態を変更しない。

| 表示 | 条件 | 操作 |
| --- | --- | --- |
| アップロード待ち | `status = UPLOADING` かつ `now <= upload_expires_at` | なし |
| 期限切れ | `status = UPLOADING` かつ `now > upload_expires_at` | 再アップロードを促す |
| 解析中 | `status = ANALYZING` かつ `now - updated_at <= 30分` | なし |
| 停滞 | `status = ANALYZING` かつ `now - updated_at > 30分` | 再解析ボタン |
| 登録完了 | `status = SUCCEEDED` | 支出詳細・編集画面へ |
| 登録対象なし | `status = NO_DATA` | 再解析ボタン |
| 解析失敗 | `status = FAILED` | `error_code` に応じたメッセージ + 再解析ボタン |

### error_codeごとの表示

| error_code | メッセージ | 再解析 |
| --- | --- | --- |
| `ANALYSIS_FAILED` | 画像を解析できませんでした | 可 |
| `INVALID_DATE` | 購入日を正しく読み取れませんでした | 可 |
| `INVALID_AMOUNT` | 金額を正しく読み取れませんでした | 可 |
| `NO_TOTAL_AMOUNT` | 合計金額を読み取れませんでした | 可 |
| `NO_DATE` | 購入日を読み取れませんでした | 可 |
| `TOO_MANY_DETAILS` | 商品明細が50件を超えています | 不可(再アップロードを促す) |
| `INTERNAL` | システムエラーが発生しました | 可 |

`NO_DATA` は解析完了だが登録対象の明細が0件だった状態。支出と月次集計には反映しない。

一時的な失敗はバックエンドが SQS の再配信で最大3回まで同じ試行をやり直す。それでも失敗したものや恒久的な失敗には再解析ボタンを表示する。

`TOO_MANY_DETAILS` は初期構成の仕様上扱えないため、再解析ボタンは出さない。

---

## API設計

### GET /months/{yyyy-MM}/analysis-requests

指定月の解析依頼を作成日時の降順で取得する。`{yyyy-MM}` が `YYYY-MM` の形式でなければ `400`。

Response:

```json
{
  "items": [
    {
      "analysis_request_id": "01JREQUESTXXX",
      "expense_id": "01JEXPENSEXXX",
      "status": "SUCCEEDED",
      "attempt": 1,
      "file_name": "receipt.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "upload_expires_at": "2026-09-15T12:15:00Z",
      "created_at": "2026-09-15T12:00:00Z",
      "updated_at": "2026-09-15T12:01:00Z"
    },
    {
      "analysis_request_id": "01JREQUESTYYY",
      "status": "NO_DATA",
      "attempt": 1,
      "file_name": "receipt-2.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "upload_expires_at": "2026-09-15T12:20:00Z",
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    },
    {
      "analysis_request_id": "01JREQUESTZZZ",
      "status": "FAILED",
      "attempt": 2,
      "file_name": "receipt-3.jpg",
      "content_type": "image/jpeg",
      "year_month": "2026-09",
      "upload_expires_at": "2026-09-15T12:20:00Z",
      "error_code": "NO_TOTAL_AMOUNT",
      "error_message": "合計金額を取得できませんでした",
      "failed_at": "2026-09-15T12:06:00Z",
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    }
  ]
}
```

| 項目 | 備考 |
| --- | --- |
| `expense_id` | `SUCCEEDED` のときだけ。支出詳細・編集画面への遷移に使う |
| `error_code` / `error_message` | `FAILED` のときだけ |
| `failed_at` | `FAILED` のときだけ。失敗日時の表示に使う |
| `upload_expires_at` | 「期限切れ」表示(`UPLOADING` かつ `now > upload_expires_at`)に使う |
| `attempt` | 現在の解析試行番号。再解析のたびに増える |
| `items` | 該当が無ければ `[]` |

値が無い項目は `null` ではなく省略する。ページネーションは持たない(1 回の Query の範囲を返す)。

#### 画面設計に対して未対応の項目

以下は画面設計にあるが API が返していない。対応するときは backend / frontend / 本書を合わせて変える。

| 項目 | 用途 | 未対応の理由 |
| --- | --- | --- |
| `store_name` / `recorded_amount` | 一覧の「店舗名」「計上額」列 | `expenses` にしか無く、一覧で支出を結合していない |

### POST /analysis-requests/{analysis_request_id}/retry

`FAILED`、`NO_DATA`、または停滞した `ANALYZING` の解析依頼に対して、新しい解析試行を開始する(再解析)。SQS による同一試行の再配信とは別の操作で、`attempt` を1増やす。

Response:

```json
{
  "analysis_request_id": "01JREQUESTYYY",
  "status": "ANALYZING",
  "attempt": 3
}
```

`status` が `FAILED` / `NO_DATA` / `ANALYZING` 以外の場合は `409 Conflict` を返す。
