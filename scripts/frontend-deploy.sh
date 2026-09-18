#!/usr/bin/env bash
set -euo pipefail

plan="${1:-frontend-plan.json}"

head_commit="$(jq -r '.headCommit' "${plan}")"
marker="$(jq -r '.marker' "${plan}")"
deploy="$(jq -r '.deploy' "${plan}")"
bucket="$(jq -r '.bucket' "${plan}")"
stack_name="$(jq -r '.stackName' "${plan}")"

echo "------------------------------------------------------------"
echo "frontend deploy: deploy=${deploy} -> ${head_commit}"
echo "  bucket: s3://${bucket}"
echo "------------------------------------------------------------"

if [[ "${deploy}" != "true" ]]; then
  echo "no frontend deploy target; updating marker only"
  aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
  exit 0
fi

aws s3 sync front-dist "s3://${bucket}" --delete
dist_id="$(aws cloudformation describe-stacks \
  --stack-name "${stack_name}" \
  --query "Stacks[0].Outputs[?OutputKey=='DistributionId'].OutputValue" \
  --output text)"
aws cloudfront create-invalidation --distribution-id "${dist_id}" --paths '/*'
aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
