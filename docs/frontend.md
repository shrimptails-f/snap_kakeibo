# フロントエンド設計

## 技術スタック

* React
* TypeScript

---

## 役割

フロントエンドは、ユーザーが画像をアップロードし、登録済みの支出と月次集計を確認するための画面を提供する。

バックエンドへの通信はAPI Gateway経由で行う。

```text
React
  |
  | HTTPS
  v
API Gateway
```

---

## 配信

ビルド成果物(`front/dist`)を S3 の frontend バケットに置き、CloudFront から OAC で配信する。詳細は [インフラ設計](./infrastructure.md#cloudfront)。

```text
npm run build
  |
  v
task front:push   S3 sync + CloudFront キャッシュ無効化
  |
  v
https://{distribution}.cloudfront.net
```

SPA のルーティングは CloudFront のエラー応答(403 / 404 → `/index.html`)で吸収する。

---

## 画面構成

画面ごとの詳細は `docs/screens` 配下に置く。

* [ダッシュボード画面](./screens/dashboard/README.md)
* [月別支出画面](./screens/monthly-expenses/README.md)
* [アップロード画面](./screens/upload/README.md)
* [アップロード履歴画面](./screens/upload-history/README.md)
* [請求詳細・編集画面](./screens/receipt-detail/README.md)

---

## 認証

ログインAPIでJWTを取得し、以降のAPIリクエストではAuthorizationヘッダーにJWTを付与する。

```http
Authorization: Bearer {JWT}
```

ログイン:

```text
POST /auth/login
```

Request:

```json
{
  "email": "user@example.com",
  "password": "password"
}
```

---

## 画像アップロード

画像アップロードはPresigned URLを使って行う。

ユーザー操作としては複数画像を一括選択できる。ただし解析は画像1枚単位で実行する。

```text
1. React
   POST /uploads

2. Backend
   upload_idを画像ごとに生成
   upload_historiesレコード作成
   S3 Presigned PUT URLを画像ごとに発行

3. React
   Presigned URLへ画像ごとにPUT
```

S3キーはクライアントから指定しない。

---

## 表示対象

初期構成で想定する表示対象:

```text
月ごとの合計

月別支出の内訳

購入明細

請求詳細

月次集計

解析ステータス

アップロード履歴
```

---

## 利用API

```text
POST /auth/login

POST /uploads

POST /uploads/{upload_id}/retry

GET /monthly-summaries

POST /monthly-summaries/{yyyy-MM}/recalculate

GET /months/{yyyy-MM}/expenses

GET /months/{yyyy-MM}/uploads

GET /billings/{billing_id}

PATCH /billings/{billing_id}
```

---

## 失敗時の表示

バックエンドは `UPLOADING` の期限切れや `ANALYZING` の停滞で状態を変更しない。画面側が `expires_at` や `updated_at` からの経過時間で表示を切り替え、再実行APIで手動回復する。

詳細は [アップロード履歴画面](./screens/upload-history/README.md) に置く。
