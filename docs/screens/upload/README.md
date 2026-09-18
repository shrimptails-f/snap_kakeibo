# アップロード画面

## 目的

複数の請求書・レシート画像をまとめてアップロードする。

ユーザー操作としては一括アップロードできるが、解析処理は画像1枚単位で進める。

---

## 画面設計

複数ファイルを選択できるアップロードUIを置く。

アップロード開始後は、画像ごとに状態を表示する。

```text
選択済み

アップロード中

解析中

完了

データなし

失敗
```

---

## 処理方針

```text
複数画像を選択
  |
  v
POST /uploads
  |
  v
画像ごとのPresigned URLを取得
  |
  v
画像ごとにS3 PUT
  |
  v
画像ごとにAI解析
```

---

## API設計

### POST /uploads

複数画像のアップロード先を作成する。

Request:

```json
{
  "files": [
    {
      "file_name": "receipt-1.jpg",
      "content_type": "image/jpeg"
    },
    {
      "file_name": "receipt-2.jpg",
      "content_type": "image/jpeg"
    }
  ]
}
```

Response:

```json
{
  "uploads": [
    {
      "upload_id": "01JUPLOADXXX",
      "s3_key": "receipts/01JUSERXXX/01JUPLOADXXX/original.jpg",
      "upload_url": "https://example.com/presigned-url",
      "expires_at": "2026-09-15T12:15:00Z"
    }
  ]
}
```

`expires_at` はPresigned URLの有効期限。期限内にPUTできなかった画像は「失敗」として表示し、再アップロードを促す。

---

## 失敗時の表示

画像ごとに以下を区別して表示する。

```text
S3へのPUT失敗
  ネットワークエラーやPresigned URL期限切れ
  再アップロードを促す

解析失敗
  アップロード履歴画面で理由を確認できる旨を表示する
```

アップロード画面は解析完了まで待たなくてよい。画面を離れた後の状態はアップロード履歴画面で追う。
