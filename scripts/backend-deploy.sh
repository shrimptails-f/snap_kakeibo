#!/usr/bin/env bash
set -euo pipefail

plan="${1:-backend-plan.json}"

head_commit="$(jq -r '.headCommit' "${plan}")"
marker="$(jq -r '.marker' "${plan}")"
application_name="$(jq -r '.applicationName' "${plan}")"
function_count="$(jq '.functions | length' "${plan}")"

echo "------------------------------------------------------------"
echo "backend deploy: ${function_count} function(s) -> ${head_commit}"
jq -r '.functions[] | "  \(.functionName)  <-  \(.imageUri)"' "${plan}"
echo "------------------------------------------------------------"

if [[ "${function_count}" == "0" ]]; then
  echo "no backend deploy target; updating marker only"
  aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
  exit 0
fi

for i in $(seq 0 $((function_count - 1))); do
  fn="$(jq -r ".functions[${i}].name" "${plan}")"
  image_uri="$(jq -r ".functions[${i}].imageUri" "${plan}")"
  function_name="$(jq -r ".functions[${i}].functionName" "${plan}")"
  deployment_group="$(jq -r ".functions[${i}].deploymentGroup" "${plan}")"

  echo "--- [$((i + 1))/${function_count}] ${function_name} ---"
  live_image_uri="$(aws lambda get-function --function-name "${function_name}" --qualifier live --query Code.ImageUri --output text)"
  if [[ "${live_image_uri}" == "${image_uri}" ]]; then
    echo "${fn}: live already uses ${image_uri}; skipping Lambda publish and CodeDeploy"
    continue
  fi

  current_version="$(aws lambda get-alias --function-name "${function_name}" --name live --query FunctionVersion --output text)"
  new_version="$(aws lambda update-function-code --function-name "${function_name}" --image-uri "${image_uri}" --publish --query Version --output text)"
  if [[ "${current_version}" == "${new_version}" ]]; then
    echo "${fn}: live already points at version ${new_version}; skipping CodeDeploy"
    continue
  fi

  appspec="$(mktemp)"
  cat >"${appspec}" <<JSON
{
  "version": 0.0,
  "Resources": [{
    "TargetService": {
      "Type": "AWS::Lambda::Function",
      "Properties": {
        "Name": "${function_name}",
        "Alias": "live",
        "CurrentVersion": "${current_version}",
        "TargetVersion": "${new_version}"
      }
    }
  }]
}
JSON

  deploy_input="$(mktemp)"
  jq -n \
    --arg applicationName "${application_name}" \
    --arg deploymentGroupName "${deployment_group}" \
    --arg content "$(jq -c . "${appspec}")" \
    '{
      applicationName: $applicationName,
      deploymentGroupName: $deploymentGroupName,
      revision: {
        revisionType: "AppSpecContent",
        appSpecContent: {content: $content}
      }
    }' >"${deploy_input}"
  deployment_id="$(aws deploy create-deployment \
    --cli-input-json "file://${deploy_input}" \
    --query deploymentId \
    --output text)"
  rm -f "${appspec}" "${deploy_input}"

  echo "${fn}: version ${current_version} -> ${new_version}, deployment ${deployment_id}"
  aws deploy wait deployment-successful --deployment-id "${deployment_id}"
  live_version="$(aws lambda get-alias --function-name "${function_name}" --name live --query FunctionVersion --output text)"
  if [[ "${live_version}" != "${new_version}" ]]; then
    echo "${fn}: live alias points to ${live_version}, want ${new_version}" >&2
    exit 1
  fi
  echo "${fn}: live -> version ${live_version} (ok)"
done

echo "------------------------------------------------------------"
echo "backend deploy done; marker ${marker} = ${head_commit}"
echo "------------------------------------------------------------"
aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
