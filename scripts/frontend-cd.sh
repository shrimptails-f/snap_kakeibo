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
elif git diff --name-status "${base_commit}" "${head_commit}" | awk '{print $2 "\n" $3}' | grep -E '^(front/|scripts/frontend-cd\.sh$)' >/dev/null; then
  deploy=true
fi

if [[ "${deploy}" != true ]]; then
  echo "no frontend deploy target; updating marker only"
  aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
  exit 0
fi

cd front
npm ci
npm run build
cd ..

aws s3 sync front/dist "s3://${stage}-${project}-front" --delete
dist_id="$(aws cloudformation describe-stacks \
  --stack-name "${stage}-${project}-app" \
  --query "Stacks[0].Outputs[?OutputKey=='DistributionId'].OutputValue" \
  --output text)"
aws cloudfront create-invalidation --distribution-id "${dist_id}" --paths '/*'
aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
