# ダッシュボード画面

## 目的

月ごとの支出合計を俯瞰する。

最初に見る画面として、家計の増減がすぐ分かることを優先する。

---

## 画面設計

月ごとの合計金額を積み上げ棒グラフで表示する。

```text
縦軸
金額

横軸
年月

棒
月ごとの支出合計
```

積み上げの内訳は、月次集計の `category_totals` を使う。

ただし月合計は `billings.final_amount` の合計を正とし、カテゴリ別内訳は購入明細ベースの補助情報として扱う。

自動解析のままのデータと、ユーザーが修正したデータを区別できるようにする。

---

## 画面遷移

棒グラフの月を押すと、月別支出画面へ遷移する。

```text
/months/2026-09
```

---

## API設計

### GET /monthly-summaries

月次集計一覧を取得する。

Response:

```json
{
  "monthly_summaries": [
    {
      "year_month": "2026-09",
      "total_amount": 128500,
      "billing_count": 25,
      "detail_count": 120,
      "category_totals": {
        "food": 86000,
        "daily_goods": 22500,
        "other": 20000,
        "unknown": 12000
      },
      "updated_at": "2026-09-15T12:01:00Z"
    }
  ]
}
```
