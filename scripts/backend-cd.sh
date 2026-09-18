#!/usr/bin/env bash
set -euo pipefail

stage="${STAGE:?STAGE is required}"
project="${PROJECT:?PROJECT is required}"
head_commit="${CODEBUILD_RESOLVED_SOURCE_VERSION:-$(git rev-parse HEAD)}"
short_sha="${head_commit:0:12}"
marker="/${stage}/${project}/cicd/backend/last-successful-commit"

api_functions=(hello upload retry-upload list-uploads get-billing)
all_functions=()
for dir in backend/cmd/*/; do
  all_functions+=("$(basename "${dir}")")
done

value_or_unset() {
  aws ssm get-parameter --name "$1" --query Parameter.Value --output text 2>/dev/null || echo "UNSET"
}

base_commit="$(value_or_unset "${marker}")"

cd backend
go test ./...
cd ..

deploy_functions=()
push_functions=()

add_deploy() {
  local name="$1"
  for existing in "${deploy_functions[@]}"; do
    [[ "${existing}" == "${name}" ]] && return 0
  done
  deploy_functions+=("${name}")
}

deploy_all_api() {
  deploy_functions=("${api_functions[@]}")
}

if [[ "${base_commit}" == "UNSET" || -z "${base_commit}" || "${base_commit}" == "None" ]]; then
  push_functions=("${all_functions[@]}")
  deploy_all_api
else
  if ! git cat-file -e "${base_commit}^{commit}" 2>/dev/null; then
    echo "base commit ${base_commit} is not available; deploying all API functions" >&2
    deploy_all_api
  else
    mapfile -t diff_lines < <(git diff --name-status "${base_commit}" "${head_commit}")
    unsafe_all=false
    changed_packages=()

    for line in "${diff_lines[@]}"; do
      [[ -z "${line}" ]] && continue
      status="$(awk '{print $1}' <<<"${line}")"
      path="$(cut -f2 <<<"${line}")"
      new_path="$(cut -f3 <<<"${line}")"

      if [[ "${path}" == *_test.go && ( -z "${new_path}" || "${new_path}" == *_test.go ) ]]; then
        continue
      fi

      case "${status}" in
        D*|R*)
          if [[ "${path}" == backend/internal/* || "${path}" == backend/cmd/* || "${new_path}" == backend/internal/* || "${new_path}" == backend/cmd/* ]]; then
            unsafe_all=true
          fi
          ;;
      esac

      if [[ "${path}" == backend/go.mod || "${path}" == backend/go.sum || "${path}" == backend/Dockerfile || "${path}" == scripts/push-image.sh || "${path}" == scripts/backend-cd.sh ]]; then
        unsafe_all=true
      elif [[ "${path}" == backend/cmd/* ]]; then
        fn="$(cut -d/ -f3 <<<"${path}")"
        for api_fn in "${api_functions[@]}"; do
          [[ "${fn}" == "${api_fn}" ]] && add_deploy "${fn}"
        done
      elif [[ "${path}" == backend/internal/* && "${path}" == *.go ]]; then
        changed_packages+=("./$(dirname "${path#backend/}")")
      fi
    done

    if [[ "${unsafe_all}" == true ]]; then
      deploy_all_api
    elif [[ ${#changed_packages[@]} -gt 0 ]]; then
      cd backend
      for pkg in "${changed_packages[@]}"; do
        if ! import_path="$(go list -f '{{.ImportPath}}' "${pkg}" 2>/dev/null)"; then
          cd ..
          deploy_all_api
          break
        fi
        for fn in "${api_functions[@]}"; do
          if go list -deps -f '{{.ImportPath}}' "./cmd/${fn}" | grep -Fxq "${import_path}"; then
            add_deploy "${fn}"
          fi
        done
      done
      [[ "$(pwd)" == */backend ]] && cd ..
    fi
  fi
  push_functions=("${deploy_functions[@]}")
fi

if [[ ${#push_functions[@]} -eq 0 && ${#deploy_functions[@]} -eq 0 ]]; then
  echo "no backend deploy target; updating marker only"
  aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
  exit 0
fi

bash scripts/push-image.sh "${stage}" "${project}" "${short_sha}" "${push_functions[@]}"

account_id="$(aws sts get-caller-identity --query Account --output text)"
region="${AWS_REGION:-$(aws configure get region || echo ap-northeast-2)}"
registry="${account_id}.dkr.ecr.${region}.amazonaws.com"
application_name="${stage}-${project}-lambda"

for fn in "${deploy_functions[@]}"; do
  image_uri="${registry}/${stage}-${project}-${fn}:${short_sha}"
  function_name="${stage}-${project}-${fn}"
  deployment_group="${stage}-${project}-${fn}-deployment-group"

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

  aws deploy wait deployment-successful --deployment-id "${deployment_id}"
  live_version="$(aws lambda get-alias --function-name "${function_name}" --name live --query FunctionVersion --output text)"
  if [[ "${live_version}" != "${new_version}" ]]; then
    echo "${fn}: live alias points to ${live_version}, want ${new_version}" >&2
    exit 1
  fi
done

aws ssm put-parameter --name "${marker}" --type String --value "${head_commit}" --overwrite >/dev/null
