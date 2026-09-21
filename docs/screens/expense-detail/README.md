# 支出詳細・編集画面

## 目的

レシート画像1枚から作成された支出と支出明細を確認し、必要に応じて修正する。

割り勘や立替回収で、最終的に自分が負担する金額(計上額)が変わるケースを扱う。

---

## 画面設計

支出単位の情報と、支出明細をまとめて表示する。

```text
支出単位
店舗名
購入日
読取金額
調整額
計上額

支出明細
商品名
カテゴリ
金額
数量
```

カテゴリは保存値ではなく表示名(`social` なら「交際・会食」)で表示する。

画像プレビューを表示できると、AIの読み取り結果の確認と修正がしやすい。

---

## 編集対象

```text
店舗名

購入日

調整額

商品名

商品金額

数量

カテゴリ
```

調整額を変更した場合は、支出単位の計上額を再計算する。調整額は符号付きで、減額は負数、増額は正数。

```text
read_amount + adjustment_amount = recorded_amount
```

計上額は返金を表現できるように負数を許容する。

---

## API設計

### GET /expenses/{expense_id}

支出単位の詳細と支出明細を取得する。利用者の支出に `expense_id` が無ければ `404`(他人の支出も同じ)。

Response:

```json
{
  "expense": {
    "expense_id": "01JEXPENSEXXX",
    "analysis_request_id": "01JREQUESTXXX",
    "store_name": "スーパー",
    "purchase_date": "2026-09-15",
    "year_month": "2026-09",
    "read_amount": 3280,
    "adjustment_amount": -500,
    "recorded_amount": 2780,
    "is_edited": false
  },
  "details": [
    {
      "detail_id": "01JITEMXXX",
      "name": "牛乳",
      "category": "food",
      "category_source": "AI",
      "amount": 281,
      "quantity": 1
    }
  ]
}
```

`details` は `detail_id`(ULID)の昇順 = 採番順。

#### 画面設計に対して未対応の項目

以下は画面設計にあるが API が返していない。対応するときは backend / frontend / 本書を合わせて変える。

| 項目 | 用途 | 未対応の理由 |
| --- | --- | --- |
| `expense.image_url` | 画像プレビュー(AI の読み取り結果と見比べて修正する) | 元画像の署名付き GET URL の発行を実装していない |
| `expense.source` / `expense.updated_at` | データの作成元・更新日時の表示 | `expenses` にはあるが返していない(支出編集 API と合わせて対応) |
| `details[].source` / `details[].is_edited` | 明細ごとの手動編集済み表示 | 同上 |

### PATCH /expenses/{expense_id}

支出単位の情報と支出明細を更新する(未実装)。

Request:

```json
{
  "store_name": "スーパー",
  "purchase_date": "2026-09-15",
  "adjustment_amount": -500,
  "details": [
    {
      "detail_id": "01JITEMXXX",
      "name": "牛乳",
      "amount": 281,
      "quantity": 1,
      "category": "food"
    }
  ]
}
```

Response:

```json
{
  "expense": {
    "expense_id": "01JEXPENSEXXX",
    "read_amount": 3280,
    "adjustment_amount": -500,
    "recorded_amount": 2780,
    "updated_at": "2026-09-15T12:10:00Z"
  }
}
```
