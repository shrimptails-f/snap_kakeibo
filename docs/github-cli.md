# GitHub CLI 利用手順

WSL の Bash で初期設定し、Dev Container と認証を共有する。`OWNER/REPO` は対象の `所有者/リポジトリ名` に置き換える。

## 1. gh を用意する

```bash
gh --version
```

WSL に未導入の場合は、[導入記事](https://qiita.com/shrimpTail/items/15be557ccd9e49717489)のインストール手順を実行する。Dev Container 側の `gh` 導入・認証共有・ラッパーの読み込みは設定済み。

## 2. PAT を作成する

[Fine-grained PAT 作成画面](https://github.com/settings/personal-access-tokens/new)で Resource owner・有効期限を指定し、**Only select repositories** で対象を選ぶ。閲覧用は他リポジトリの参照が必要な場合に作成する。

| 設定 | 開発用（ghp） | 閲覧用（ghw） |
| --- | --- | --- |
| 対象 | このリポジトリのみ | 参照するリポジトリ |
| Contents | Read and write | Read-only |
| Issues | Read and write | Read-only |
| Pull requests | Read and write | Read-only |
| Actions | Read-only | Read-only |

Metadata は自動付与。`.github/workflows/` の変更を push する場合のみ、開発用に Workflows: Read and write を追加する。

## 3. 開発用 PAT を保存する

WSL で実行し、入力欄に開発用 PAT を貼り付ける。

```bash
set +x
(
  umask 077
  read -rsp '開発用 PAT: ' input_token; printf '\n'
  [ -n "$input_token" ] || exit 1
  printf '%s' "$input_token" |
    env -u GH_TOKEN -u GITHUB_TOKEN gh auth login \
      --hostname github.com --git-protocol https \
      --with-token --insecure-storage
)
```

保存成功後、環境変数を外して確認する。

```bash
unset GH_TOKEN GITHUB_TOKEN
gh auth status
```

PAT は `~/.config/gh` に平文保存される。トークンや認証ファイルの内容を Git・チャット・ログに載せない。

## 4. 閲覧用 PAT を保存する（任意）

WSL で実行する。閲覧用 PAT は `gh auth login` で登録しない。

```bash
set +x
(
  umask 077
  mkdir -p ~/.config/gh
  read -rsp '閲覧用 PAT: ' read_token; printf '\n'
  [ -n "$read_token" ] || exit 1
  printf '%s\n' "$read_token" > ~/.config/gh/read-token
  chmod 600 ~/.config/gh/read-token
)
```

## 5. ラッパーを読み込んで使う

このリポジトリ内の Bash で実行する。非対話シェルでは、コマンドを実行するシェルごとに読み込む。

```bash
source "$(git rev-parse --show-toplevel)/scripts/gh-wrappers.bash"
ghp pr list --state closed
ghw pr list -R OWNER/REPO
```

- `ghp`：このリポジトリの参照・依頼に必要な書き込み。
- `ghw`：他リポジトリの読み取り。認証エラー時に別の認証へ切り替えない。
- WSL の対話 Bash で常時使う場合は、`~/.bashrc` に `source "/リポジトリの絶対パス/scripts/gh-wrappers.bash"` を一度追加し、`source ~/.bashrc` で反映する。
- AI の操作ルールは [AGENTS.md](../AGENTS.md)、関数の実装は [scripts/gh-wrappers.bash](../scripts/gh-wrappers.bash) を参照。

## 6. Dev Container に反映する

WSL から VS Code で開き、**Dev Containers: Rebuild Container** を実行する。コンテナ内で確認する。

```bash
gh --version
ghp pr list --state closed
# 閲覧用 PAT を設定した場合
ghw pr list -R OWNER/REPO
```

WSL の `~/.config/gh` は `/home/dev/.config/gh` に読み取り専用で共有される。再入力は不要。PAT の期限切れ・更新時は、WSL 側で手順3・4を再実行する。

## Git の push にも PAT を使う場合

このリポジトリ内で `OWNER/REPO` を置き換えて実行する。

```bash
git remote set-url origin https://github.com/OWNER/REPO.git
ghp auth setup-git --hostname github.com
git ls-remote origin HEAD
```

`ghp`／`ghw` は通常の `git` の認証を切り替えない。既存の SSH 鍵の権限も変わらない。読み取り専用マウントは設定ファイルへの書き込みを防ぐもので、GitHub の操作権限は PAT に従う。

## 参考記事

- [WSL と Dev Container で GitHub CLI を使う — リポジトリ限定 PAT と認証の永続化](https://qiita.com/shrimpTail/items/15be557ccd9e49717489)
- [AI に GitHub CLI を使わせる — 2種類の PAT と AGENTS.md による操作ルール](https://qiita.com/shrimpTail/items/e47687af08cf8910d128)
