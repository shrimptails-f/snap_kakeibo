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
| `TEXTRACT_FAILED` | 画像を解析できませんでした | 可 |
| `TEXTRACT_PARTIAL_SUCCESS` | 画像の一部を解析できませんでした | 可 |
| `NO_TOTAL_AMOUNT` | 合計金額を読み取れませんでした | 可 |
| `NO_DATE` | 購入日を読み取れませんでした | 可 |
| `TOO_MANY_DETAILS` | 商品明細が50件を超えています | 不可(再アップロードを促す) |
| `INTERNAL` | システムエラーが発生しました | 可 |

`NO_DATA` は解析完了だが登録対象の明細が0件だった状態。請求と月次集計には反映しない。

Textractジョブ失敗と部分成功は自動で最大3回まで再実行する。3回使い切った後も、手動再実行ボタンは表示する。

`TOO_MANY_DETAILS` は初期構成の仕様上扱えないため、再実行ボタンは出さない。

---

## API設計

### GET /months/{yyyy-MM}/uploads

指定月のアップロード履歴を取得する。

Response:

```json
{
  "year_month": "2026-09",
  "uploads": [
    {
      "upload_id": "01JUPLOADXXX",
      "billing_id": "01JBILLXXX",
      "status": "SUCCEEDED",
      "attempt": 1,
      "file_name": "receipt.jpg",
      "store_name": "スーパー",
      "final_amount": 2780,
      "expires_at": "2026-09-15T12:15:00Z",
      "error_code": null,
      "error_message": null,
      "failed_at": null,
      "created_at": "2026-09-15T12:00:00Z",
      "updated_at": "2026-09-15T12:01:00Z"
    },
    {
      "upload_id": "01JUPLOADYYY",
      "billing_id": null,
      "status": "NO_DATA",
      "attempt": 1,
      "file_name": "receipt-2.jpg",
      "store_name": null,
      "final_amount": null,
      "expires_at": "2026-09-15T12:20:00Z",
      "error_code": null,
      "error_message": null,
      "failed_at": null,
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    },
    {
      "upload_id": "01JUPLOADZZZ",
      "billing_id": null,
      "status": "FAILED",
      "attempt": 2,
      "file_name": "receipt-3.jpg",
      "store_name": null,
      "final_amount": null,
      "expires_at": "2026-09-15T12:20:00Z",
      "error_code": "NO_TOTAL_AMOUNT",
      "error_message": "合計金額を取得できませんでした",
      "failed_at": "2026-09-15T12:06:00Z",
      "created_at": "2026-09-15T12:05:00Z",
      "updated_at": "2026-09-15T12:06:00Z"
    }
  ]
}
```

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
