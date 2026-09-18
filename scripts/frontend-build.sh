#!/usr/bin/env bash
set -euo pipefail

stage="${STAGE:?STAGE is required}"
project="${PROJECT:?PROJECT is required}"
head_commit="${CODEBUILD_RESOLVED_SOURCE_VERSION:-$(git rev-parse HEAD)}"
marker="/${stage}/${project}/cicd/frontend/last-successful-commit"

value_or_unset() {
  aws ssm get-parameter --name "$1" --query Parameter.Value --output text 2>/dev/null || echo "UNSET"
}

base_commit="$(value_or_unset "${marker}")"
deploy=false

if [[ "${base_commit}" == "UNSET" || -z "${base_commit}" || "${base_commit}" == "None" ]]; then
  deploy=true
elif ! git cat-file -e "${base_commit}^{commit}" 2>/dev/null; then
  echo "base commit ${base_commit} is not available; deploying frontend" >&2
  deploy=true
elif git diff --name-status "${base_commit}" "${head_commit}" | awk '{print $2 "\n" $3}' | grep -E '^(front/|scripts/frontend-build\.sh$|scripts/frontend-deploy\.sh$)' >/dev/null; then
  deploy=true
fi

mkdir -p build/scripts
cp scripts/frontend-deploy.sh build/scripts/frontend-deploy.sh

if [[ "${deploy}" == true ]]; then
  cd front
  pnpm install --frozen-lockfile
  pnpm build
  cd ..
  mkdir -p build/front-dist
  cp -R front/dist/. build/front-dist/
fi

jq -n \
  --arg headCommit "${head_commit}" \
  --arg marker "${marker}" \
  --arg bucket "${stage}-${project}-front" \
  --arg stackName "${stage}-${project}-app" \
  --argjson deploy "${deploy}" \
  '{headCommit: $headCommit, marker: $marker, deploy: $deploy, bucket: $bucket, stackName: $stackName}' \
  > build/frontend-plan.json
