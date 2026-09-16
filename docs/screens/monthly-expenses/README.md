# 月別支出画面

## 目的

指定月の細かい支出を確認する。

その月に何へ多く使ったかを、視覚的に把握できることを優先する。

---

## 画面設計

上部に円グラフを表示する。

下部に購入明細の棒グラフを金額降順で表示する。

```text
上部
月の内訳を円グラフで表示

下部
購入したものを金額降順の棒グラフで表示
```

購入明細は表示上限なし、ページネーションなしで表示する。

月合計は請求単位の `billings.final_amount` を正とする。

円グラフの内訳は購入明細の `category` と `amount` から作る。Textract由来のカテゴリは参考値として扱い、ユーザー修正後の値を優先する。

---

## 表示項目

```text
商品名

購入金額

購入日

店舗名

請求ID
```

---

## 画面遷移

購入明細を押すと、その明細が含まれる請求詳細・編集画面を開く。

詳細表示は別画面またはモーダルのどちらでも実装可能とする。

---

## API設計

### GET /months/{yyyy-MM}/expenses

指定月の購入明細を取得する。

Response:

```json
{
  "year_month": "2026-09",
  "items": [
    {
      "detail_id": "01JITEMXXX",
      "billing_id": "01JBILLXXX",
      "name": "牛乳",
      "category": "food",
      "amount": 281,
      "quantity": 1,
      "source": "TEXTRACT",
      "is_edited": false,
      "store_name": "スーパー",
      "purchased_at": "2026-09-15"
    }
  ]
}
```
