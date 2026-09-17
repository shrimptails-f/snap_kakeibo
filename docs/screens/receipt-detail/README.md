# 請求詳細・編集画面

## 目的

請求書・レシート画像1枚から作成された請求データと購入明細を確認し、必要に応じて修正する。

割り勘や手動値引きで、最終的に自分が負担する金額が変わるケースを扱う。

---

## 画面設計

請求単位の情報と、購入した商品をまとめて表示する。

```text
請求単位
店舗名
購入日
元の合計金額
値引き額
最終金額

購入明細
商品名
金額
数量
```

画像プレビューを表示できると、AIの読み取り結果の確認と修正がしやすい。

---

## 編集対象

```text
店舗名

購入日

値引き額

商品名

商品金額

数量
```

値引き額を変更した場合は、請求単位の最終金額を再計算する。

```text
original_amount - discount_amount = final_amount
```

---

## API設計

### GET /billings/{billing_id}

請求単位の詳細と購入明細を取得する。

Response:

```json
{
  "bill": {
    "billing_id": "01JBILLXXX",
    "upload_id": "01JUPLOADXXX",
    "store_name": "スーパー",
    "purchased_at": "2026-09-15",
    "original_amount": 3280,
    "discount_amount": 500,
    "final_amount": 2780,
    "source": "AI",
    "is_edited": false,
    "image_url": "https://example.com/presigned-get-url",
    "updated_at": "2026-09-15T12:01:00Z"
  },
  "items": [
    {
      "detail_id": "01JITEMXXX",
      "name": "牛乳",
      "amount": 281,
      "quantity": 1,
      "source": "AI",
      "is_edited": false
    }
  ]
}
```

### PATCH /billings/{billing_id}

請求単位の情報と購入明細を更新する。

Request:

```json
{
  "store_name": "スーパー",
  "purchased_at": "2026-09-15",
  "discount_amount": 500,
  "items": [
    {
      "detail_id": "01JITEMXXX",
      "name": "牛乳",
      "amount": 281,
      "quantity": 1
    }
  ]
}
```

Response:

```json
{
  "bill": {
    "billing_id": "01JBILLXXX",
    "original_amount": 3280,
    "discount_amount": 500,
    "final_amount": 2780,
    "updated_at": "2026-09-15T12:10:00Z"
  }
}
```
