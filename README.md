# snap_kakeibo 開発環境

VS Code の Dev Containers でこのディレクトリを開き、**Reopen in Container** を実行します。Docker Desktop を WSL で使う場合は、対象ディストリビューションの WSL integration を有効にしてください。初回起動時に `task setup` が走ります。

Go 1.26、Node.js/pnpm（React + TypeScript 用）、Task、Codex CLI、Claude Code を Alpine ベースの開発コンテナへ導入します。モノレポは直下に `backend/`（Go）、`front/`（React + TypeScript）、`infra/`（インフラ資材）を置く前提です。アプリ本体はまだ作成していません。React アプリを作る場合はルートで `pnpm create vite@latest front --template react-ts` を実行し、`cd front && pnpm install` してください。Go モジュールは `backend/` で `go mod init <module-path>` を実行してください。

`task setup` は `backend/go.mod` があれば Go の依存関係を、`front/package.json` があれば pnpm の依存関係を導入します。

`front/pnpm-workspace.yaml` の `minimumReleaseAge: 2880` により、`pnpm install` / `pnpm add` / `pnpm update` は公開から 48 時間経っていない版を(推移的依存も含めて)選びません。CI の Safe Chain も同じ 48 時間ルールでダウンロードをブロックするので、値を変えるときは両方揃えてください。`pnpm install --frozen-lockfile`(`task setup` / CI / CD)は lockfile をそのまま入れるだけで再解決しません。`task go:test` は `backend/`、`task web:dev` / `task web:build` / `task web:test`(vitest)は `front/` で実行します。

コンテナは UID/GID 1000 の `dev` で起動し、Dockerfile の最終ユーザーも `dev` です。`sudo` と Docker ソケットのマウントは設けていません。Codex と Claude の認証情報は各専用ボリュームに保存します。ホストの UID/GID が 1000 以外なら、Compose の `user` と Dockerfile のユーザー作成値を合わせて変更してください。

ホストの `${HOME}/.aws` をコンテナの `/home/dev/.aws` に読み取り専用でマウントします。ホストで `AWS_PROFILE` と `AWS_REGION` を指定して起動すればその値を使い、未指定なら `default` と `ap-northeast-2` を使います(リージョンの経緯は `docs/infra/infrastructure.md` を参照)。AWS CLI も導入しています。SSO プロファイルの更新など、`.aws` への書き込みが必要なログイン操作はホスト側で実行してください。

Floci を同じ Compose 構成で起動し、開発コンテナ内の標準の AWS 接続先を `http://floci:4566` にします。DynamoDB と S3 を使う際は `task floci:check` で疎通を確認できます。このタスクは Floci 用のダミー認証情報を使います。AWS SDK のコードでも `AWS_ENDPOINT_URL` を読み取り、DynamoDB/S3 クライアントのエンドポイントを設定してください。ホスト PC からは `http://localhost:4566` に接続できます。

実 AWS を使うコマンドでは、`AWS_ENDPOINT_URL` を外してホストのプロファイルを使います。例: `env -u AWS_ENDPOINT_URL aws sts get-caller-identity`。Floci のデータは `floci-data` ボリュームに保存するため、コンテナを再作成しても保持します。

## ローカルで画面を動かす

API Gateway と Lambda の代わりに `backend/tools/localapi` が HTTP を受け、`backend/cmd/*` の各 Lambda を子プロセス(aws-lambda-go のローカル RPC モード)として呼び出します。DynamoDB / S3 / SQS / SSM は Floci を使うので、実 AWS には繋ぎません。

```bash
task local:seed   # 初回のみ。Floci にテーブル・バケット・キュー・JWT 署名鍵、利用者(dev@example.com / password)、画面確認用のサンプルを作る
task local:api    # :8080 で API を待ち受ける(Ctrl+C で停止)
task web:dev      # 別の端末で。front/.env の VITE_DEV_API_PROXY=http://localhost:8080 で /api を転送する
```

`front/.env` が無ければ `front/.env.example` をコピーしてください。ブラウザで `http://localhost:5173` を開き、seed した利用者でログインします。利用者を変えるときは `task local:seed EMAIL=... PASSWORD=...` で上書きできます。

- Lambda のログは `[auth-login]` のように関数名を付けて `task local:api` の端末に出ます。`LOG_LEVEL=debug task local:api` で詳細を出せます。
- サンプルは今月・先月・先々月の解析依頼と支出(今月は `SUCCEEDED` / `NO_DATA` / `FAILED` / `ANALYZING` / `UPLOADING` の全状態)で、本番と同じ upload / analysis の usecase で登録します(OpenAI だけ固定応答)。ID が固定なので `task local:seed` を繰り返しても増えません。月が変わると今月分が新しく追加され、前の月の分は残ります。サンプルを入れたくない場合は `cd backend && go run ./tools/localseed -samples=false` を使ってください。
- 画像のアップロード(presigned URL への PUT)は `http://localhost:8080/s3/...` を経由して Floci に入ります。解析 Lambda(`analyze-receipt`)は動かさないため、自分でアップロードしたものは `UPLOADING` のまま残ります。
- `cmd/` のコードを変えたら `task local:api` を起動し直します(起動時に `go build` します)。
- Floci のデータを消してやり直すときは `floci-data` ボリュームを削除してから `task local:seed` を再実行します。

ワークスペースはホストとのバインドマウントです。Codex と Claude の設定、Go モジュールキャッシュは名前付きボリュームでコンテナを作り直しても保持します。ホストに `${HOME}/.aws` が存在することを起動前に確認してください。

Alpine では Claude Code に `libgcc`、`libstdc++`、`ripgrep` と `USE_BUILTIN_RIPGREP=0` が必要です。Codex/Claude はコンテナで `codex` / `claude` として起動し、初回は各サービスへのログインが必要です。`task doctor` でツールと実行ユーザーを確認できます。アプリのポート 5173 と 8080 は VS Code Dev Containers で転送します。

Bash のプロンプトには、Git リポジトリ内であれば現在のブランチ名を青色で表示します。

## GitHub CLI (`gh`)

開発コンテナに Alpine の `github-cli` を導入します。既存のコンテナでは VS Code の **Dev Containers: Rebuild Container** を実行し、`gh --version` または `task doctor` で確認してください。

権限をリポジトリ単位に限定するため、[Fine-grained personal access token の作成画面](https://github.com/settings/personal-access-tokens/new)で Resource owner を選び、Repository access を **Only select repositories** にしてこのリポジトリだけを選択します。有効期限はまず 30 日を目安に設定します。Organization のポリシーによっては管理者の承認が必要です。

Repository permissions は使う操作に合わせて設定します。最初は閲覧用で始め、必要な書き込み権限を追加してください。

| 操作 | 権限の目安 |
| --- | --- |
| コード・PR の閲覧 | Contents: Read-only、Pull requests: Read-only |
| PR の作成・編集・レビュー | Pull requests: Read and write |
| この PAT で HTTPS の Git push | Contents: Read and write |
| Issue の閲覧／作成・編集 | Issues: Read-only／Read and write |
| Actions の実行結果・ログの閲覧 | Actions: Read-only |
| Actions の手動実行・再実行 | Actions: Read and write |
| この PAT で `.github/workflows/` を更新 | Contents と Workflows: Read and write |

Metadata の読み取り権限は自動で含まれます。追加オプションや API によって別の権限が必要になるため、詳細は [GitHub の権限一覧](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)を確認してください。PR 作成時にブランチも push するなら、Git 側の書き込み認証も必要です。ブランチへの直接 push やマージの制限は GitHub の ruleset / branch protection で設定します。

WSL ホストの `${HOME}/.config/gh` をコンテナの `/home/dev/.config/gh` に読み取り専用でマウントし、ホストに保存済みの `gh` 認証を共有します。ホストでこのリポジトリ専用の PAT を保存済みなら、コンテナや端末を再起動してもトークンの再入力は不要です。起動前にホストの設定ディレクトリが存在することを確認してください。この方式はトークンが設定ファイルに保存されている場合に利用できます。ホストの OS 資格情報ストアに保存したトークンは、このマウントだけでは共有できません。

設定変更後は **Dev Containers: Rebuild Container** を実行し、コンテナ内で確認します。

```bash
gh auth status
gh pr list --state closed
```

ログイン・トークン更新・ログアウトなど、共有した `gh` 設定への書き込みはホスト側で行います。ホスト側の認証変更はコンテナにも反映されます。読み取り専用なのは設定ファイルであり、GitHub 上の操作権限は PAT の設定に従います。コンテナ内のプロセスもこの認証を利用できるため、共有する認証はこのリポジトリに限定してください。トークンをチャット、Dockerfile、Compose の値、Git 管理ファイルへ貼り付けないでください。

保存済み認証を一時的に上書きする場合のみ、コンテナ内の Bash で別の PAT を `GH_TOKEN` に入力できます。以下はトークンそのものをシェル履歴に残しません。`set -x` によるトレースが有効な場合は、先に `set +x` で無効にしてください。

```bash
read -rsp 'GitHub token: ' GH_TOKEN; printf '\n'
export GH_TOKEN
gh auth status
gh pr list
```

`GH_TOKEN` があれば保存済み認証より優先され、`gh auth login` は不要です。Fine-grained PAT にはこの渡し方が[公式にも推奨されています](https://cli.github.com/manual/gh_auth_login)。この一時的な上書きはその端末と子プロセスだけに適用され、ファイルには保存されません。終了時は `unset GH_TOKEN` で保存済み認証に戻せますが、既に起動した子プロセスの環境変数は残るため、必要に応じて終了してください。

HTTPS の Git にもこの認証を使う場合は `gh auth setup-git` を実行します。SSH の Git 認証は別管理です。ホストからの Git 資格情報転送や SSH agent 転送がある場合、PAT の権限だけではコンテナ全体の Git 操作を制限できません。
