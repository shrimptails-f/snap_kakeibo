#!/bin/bash
# Status line converted from ~/.bashrc PS1:
#   export PS1='\u@\h:\w\[\033[34m\]$(git_prompt_branch)\[\033[0m\] \$ '
# (trailing "\$ " prompt character removed for the status line)
# Uses python3 for JSON parsing (jq is not installed in this container).

input=$(cat)

# Emit: cwd, session_name, five_hour%, seven_day%, one per line ("" when absent)
readarray -t fields < <(printf '%s' "$input" | python3 -c '
import json, sys
d = json.load(sys.stdin)
rl = d.get("rate_limits") or {}
def pct(k):
    v = (rl.get(k) or {}).get("used_percentage")
    return "" if v is None else f"{round(v)}"
print(d.get("cwd") or "")
print(d.get("session_name") or "")
print(pct("five_hour"))
print(pct("seven_day"))
')
cwd=${fields[0]}
session_name=${fields[1]}
five=${fields[2]}
week=${fields[3]}

user=$(whoami)
host=$(hostname -s)

branch=$(git -C "$cwd" --no-optional-locks symbolic-ref --quiet --short HEAD 2>/dev/null)
git_part=""
[ -n "$branch" ] && git_part=" ($branch)"

session_part=""
[ -n "$session_name" ] && session_part=" [$session_name]"

usage_part=""
[ -n "$five" ] && usage_part="$usage_part 5h:${five}%"
[ -n "$week" ] && usage_part="$usage_part 7d:${week}%"

# session name: bright cyan, usage: bright yellow
printf '%s@%s:%s\033[34m%s\033[0m\033[96m%s\033[0m\033[93m%s\033[0m' "$user" "$host" "$cwd" "$git_part" "$session_part" "$usage_part"
