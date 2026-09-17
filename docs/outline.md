# 家計簿アプリ AWS設計

## コンセプト

画像をアップロードするだけで、レシート・請求書を自動解析して家計簿へ登録する。

* 個人利用前提
* インターネット公開
* 手動登録済みユーザーのみ利用可能
* AWSコストをできる限り抑える
* 画像1枚単位で非同期解析
* 購入明細は1画像あたり最大50件まで扱う
* 月次集計を自動更新する

---

## 設計ドキュメント

詳細設計は領域ごとに分割する。

* [インフラ設計](./infrastructure.md)
* [バックエンド設計](./backend.md)
* [フロントエンド設計](./frontend.md)
* [DB設計](./database.md)

画面ごとの設計は以下に置く。

* [ダッシュボード画面](./screens/dashboard/README.md)
* [月別支出画面](./screens/monthly-expenses/README.md)
* [アップロード画面](./screens/upload/README.md)
* [アップロード履歴画面](./screens/upload-history/README.md)
* [請求詳細・編集画面](./screens/receipt-detail/README.md)

---

## 技術スタック

### Frontend

* React
* TypeScript

### Backend

* Go
* AWS Lambda
* Amazon API Gateway
* OpenAI API

### Infrastructure

* AWS CDK for Go

### AWS

* S3
* API Gateway
* Lambda
* DynamoDB
* SNS
* SQS
* CloudWatch Alarm
* SSM Parameter Store

---

## 全体構成

```text
React
  |
  | HTTPS
  v
API Gateway
  |
  v
Lambda
  |
  +----------------------------+
  |                            |
  | Presigned URL発行          | Login / 家計簿API
  v                            v
S3                         DynamoDB
  |
  | ObjectCreated
  v
SQS (Analyze Queue) ----> DLQ
  |
  v
Analyze Lambda
  |
  | 画像を渡して読み取り + 明細カテゴリ分類
  v
OpenAI API
  |
  v
DynamoDB

DLQ ---> CloudWatch Alarm ---> SNS ---> メール通知
```

---

## 設計方針

```text
低コスト

Serverless

Event Driven

非同期処理

冪等性保証

DynamoDB Atomic Update

DynamoDB Transaction

失敗は upload_histories に理由付きで残す

月次集計 = monthly_summaries

請求書・レシート画像1枚 = upload_histories 1件

請求単位の支出 = billings 1件

購入商品 = billing_details
```

---

## 採用しないもの

初期構成では以下は使用しない。

```text
Cognito
Step Functions
RDS
ECS
EventBridge Scheduler
```

理由:

```text
個人利用ではオーバースペック

Lambda + DynamoDB中心の構成で十分

固定費を極力持たない
```

SQSはStep Functionsのようなオーケストレーションには使わないが、非同期処理のバッファとDLQとして使う。無料枠内で収まり、失敗イベントが消えずに残るため。

停滞検知の定期実行(EventBridge Scheduler)は持たず、画面側の経過時間判定と手動再実行で対応する。
