# snap_kakeibo 開発環境

VS Code の Dev Containers でこのディレクトリを開き、**Reopen in Container** を実行します。Docker Desktop を WSL で使う場合は、対象ディストリビューションの WSL integration を有効にしてください。初回起動時に `task setup` が走ります。

Go 1.26、Node.js/pnpm（React + TypeScript 用）、Task、Codex CLI、Claude Code を Alpine ベースの開発コンテナへ導入します。モノレポは直下に `backend/`（Go）、`front/`（React + TypeScript）、`infra/`（インフラ資材）を置く前提です。アプリ本体はまだ作成していません。React アプリを作る場合はルートで `pnpm create vite@latest front --template react-ts` を実行し、`cd front && pnpm install` してください。Go モジュールは `backend/` で `go mod init <module-path>` を実行してください。

`task setup` は `backend/go.mod` があれば Go の依存関係を、`front/package.json` があれば pnpm の依存関係を導入します。

`front/pnpm-workspace.yaml` の `minimumReleaseAge: 2880` により、`pnpm install` / `pnpm add` / `pnpm update` は公開から 48 時間経っていない版を(推移的依存も含めて)選びません。CI の Safe Chain も同じ 48 時間ルールでダウンロードをブロックするので、値を変えるときは両方揃えてください。`pnpm install --frozen-lockfile`(`task setup` / CI / CD)は lockfile をそのまま入れるだけで再解決しません。`task go:test` は `backend/`、`task web:dev` / `task web:build` / `task web:test`(vitest)は `front/` で実行します。

コンテナは UID/GID 1000 の `dev` で起動し、Dockerfile の最終ユーザーも `dev` です。`sudo` と Docker ソケットのマウントは設けていません。Codex と Claude の認証情報は各専用ボリュームに保存します。ホストの UID/GID が 1000 以外なら、Compose の `user` と Dockerfile のユーザー作成値を合わせて変更してください。

ホストの `${HOME}/.aws` をコンテナの `/home/dev/.aws` に読み取り専用でマウントします。ホストで `AWS_PROFILE` と `AWS_REGION` を指定して起動すればその値を使い、未指定なら `default` と `ap-northeast-2` を使います(リージョンの経緯は `docs/infra/infrastructure.md` を参照)。AWS CLI も導入しています。SSO プロファイルの更新など、`.aws` への書き込みが必要なログイン操作はホスト側で実行してください。

Floci を同じ Compose 構成で起動し、開発コンテナ内の標準の AWS 接続先を `http://floci:4566` にします。DynamoDB と S3 を使う際は `task floci:check` で疎通を確認できます。このタスクは Floci 用のダミー認証情報を使います。AWS SDK のコードでも `AWS_ENDPOINT_URL` を読み取り、DynamoDB/S3 クライアントのエンドポイントを設定してください。ホスト PC からは `http://localhost:4566` に接続できます。

実 AWS を使うコマンドでは、`AWS_ENDPOINT_URL` を外してホストのプロファイルを使います。例: `env -u AWS_ENDPOINT_URL aws sts get-caller-identity`。Floci のデータは `floci-data` ボリュームに保存するため、コンテナを再作成しても保持します。

ワークスペースはホストとのバインドマウントです。Codex と Claude の設定、Go モジュールキャッシュは名前付きボリュームでコンテナを作り直しても保持します。ホストに `${HOME}/.aws` が存在することを起動前に確認してください。

Alpine では Claude Code に `libgcc`、`libstdc++`、`ripgrep` と `USE_BUILTIN_RIPGREP=0` が必要です。Codex/Claude はコンテナで `codex` / `claude` として起動し、初回は各サービスへのログインが必要です。`task doctor` でツールと実行ユーザーを確認できます。アプリのポート 5173 と 8080 は VS Code Dev Containers で転送します。

Bash のプロンプトには、Git リポジトリ内であれば現在のブランチ名を青色で表示します。
