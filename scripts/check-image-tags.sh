#!/usr/bin/env bash
# App スタックの deploy 前に、全関数のイメージタグ SSM パラメータが設定済みか確認する。
# UNSET のまま deploy すると Lambda の作成が ECR のイメージ不在で失敗するため、先に止める。
#
#   scripts/check-image-tags.sh <stage> <project>
set -euo pipefail

stage="${1:-}"
project="${2:-}"
if [[ -z "${stage}" || -z "${project}" ]]; then
  echo "usage: $0 <stage> <project>" >&2
  exit 1
fi

status=0
for dir in backend/cmd/*/; do
  [[ -f "${dir}/main.go" ]] || continue
  function_name="$(basename "${dir}")"
  parameter_name="/${stage}/${project}/functions/${function_name}/image-tag"
  value="$(aws ssm get-parameter --name "${parameter_name}" --query Parameter.Value --output text 2>/dev/null || true)"
  if [[ -z "${value}" || "${value}" == "None" || "${value}" == "UNSET" ]]; then
    echo "NG ${function_name}: ${parameter_name} is not set. Run: FUNCTIONS=${function_name} task image:push" >&2
    status=1
  else
    echo "ok ${function_name}: ${value}"
  fi
done
exit "${status}"
