# Source from Bash; authentication files are never printed.
ghp() (
  set +x
  unset GH_TOKEN GITHUB_TOKEN
  command gh "$@"
)

ghw() (
  set +x
  local read_token
  local token_file="${HOME}/.config/gh/read-token"
  if [[ ! -r "$token_file" ]]; then
    printf 'ghw: 読み取り用 PAT を ~/.config/gh/read-token に保存してください。\n' >&2
    return 1
  fi
  read_token=$(< "$token_file")
  if [[ -z "${read_token//[[:space:]]/}" ]]; then
    printf 'ghw: 読み取り用 PAT が空です。\n' >&2
    return 1
  fi
  export GH_TOKEN="$read_token"
  unset GITHUB_TOKEN
  command gh "$@"
)
