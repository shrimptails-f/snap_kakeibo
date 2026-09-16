# Show the current Git branch in interactive Bash prompts.
git_prompt_branch() {
  local branch
  branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null)" || return
  printf ' (%s)' "$branch"
}

export PS1='\u@\h:\w\[\033[34m\]$(git_prompt_branch)\[\033[0m\] \$ '
