# Backend Directory Structure

```text
backend/                    // バックエンドルート
├── cmd/                    // エントリポイント
│   └── {api名}/            // Lambda単位
│       └── main.go         // 起動処理
├── internal/               // 内部実装
│   ├── common/             // 共通ドメイン
│   │   ├── domain/         // 値オブジェクト
│   │   └── const.go        // 共通常量
│   ├── {業務名}/           // 業務単位
│   │   ├── application/    // ユースケース
│   │   ├── domain/         // 業務モデル
│   │   ├── infrastructure/ // 外部接続
│   │   └── library/        // 業務内共通
│   ├── library/            // 共通ラッパー
│   │   └── {ライブラリ名}/  // ライブラリ単位
│   └── di/                 // 依存注入
│       ├── di.go           // コンテナ
│       └── {API名}.go      // API別定義
├── test/                   // 結合テスト
├── docs/                   // 資料
├── go.mod                  // Goモジュール
├── go.sum                  // 依存チェックサム
└── README.md               // 概要
```

```text
application/            // ユースケース
└── {業務名}_usecase.go // 業務処理
```

```text
domain/           / 業務モデル
└── {モデル名}.go // モデル定義
```

```text
infrastructure/                   // 外部接続
├── {テーブル名}_repository.go     // DB実装
└── {リソース名}_infrastructure.go // AWS等実装
```

```text
library/             // 薄いラッパー
└── {ライブラリ名}/   // 対象単位
    ├── client.go    // クライアント
    └── interface.go // インターフェース
```
