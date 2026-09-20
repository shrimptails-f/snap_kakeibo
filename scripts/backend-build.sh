#!/usr/bin/env bash
set -euo pipefail

stage="${STAGE:?STAGE is required}"
project="${PROJECT:?PROJECT is required}"
head_commit="${CODEBUILD_RESOLVED_SOURCE_VERSION:-$(git rev-parse HEAD)}"
short_sha="${head_commit:0:12}"
marker="/${stage}/${project}/cicd/backend/last-successful-commit"

# backend/cmd 配下の全関数が live Alias + CodeDeploy の対象(analyze-receipt も SQS を Alias に付けている)
all_functions=()
for dir in backend/cmd/*/; do
  [[ -f "${dir}/main.go" ]] || continue
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

deploy_all() {
  deploy_functions=("${all_functions[@]}")
}

if [[ "${base_commit}" == "UNSET" || -z "${base_commit}" || "${base_commit}" == "None" ]]; then
  push_functions=("${all_functions[@]}")
  deploy_all
else
  if ! git cat-file -e "${base_commit}^{commit}" 2>/dev/null; then
    echo "base commit ${base_commit} is not available; deploying all functions" >&2
    deploy_all
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

      if [[ "${path}" == backend/go.mod || "${path}" == backend/go.sum || "${path}" == backend/Dockerfile || "${path}" == scripts/push-image.sh || "${path}" == scripts/backend-build.sh || "${path}" == scripts/backend-deploy.sh ]]; then
        unsafe_all=true
      elif [[ "${path}" == backend/cmd/* ]]; then
        fn="$(cut -d/ -f3 <<<"${path}")"
        for known_fn in "${all_functions[@]}"; do
          [[ "${fn}" == "${known_fn}" ]] && add_deploy "${fn}"
        done
      elif [[ "${path}" == backend/internal/* && "${path}" == *.go ]]; then
        changed_packages+=("./$(dirname "${path#backend/}")")
      fi
    done

    if [[ "${unsafe_all}" == true ]]; then
      deploy_all
    elif [[ ${#changed_packages[@]} -gt 0 ]]; then
      cd backend
      for pkg in "${changed_packages[@]}"; do
        if ! import_path="$(go list -f '{{.ImportPath}}' "${pkg}" 2>/dev/null)"; then
          cd ..
          deploy_all
          break
        fi
        for fn in "${all_functions[@]}"; do
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

# 対象をログで一目で分かるようにまとめて出す
list_or_none() { if [[ $# -eq 0 ]]; then echo "  (none)"; else printf '  %s\n' "$@"; fi; }
echo "------------------------------------------------------------"
echo "backend deploy plan"
echo "  base : ${base_commit}"
echo "  head : ${head_commit} (image tag ${short_sha})"
echo "ECR push:"
list_or_none "${push_functions[@]}"
echo "CodeDeploy (live alias):"
list_or_none "${deploy_functions[@]}"
echo "------------------------------------------------------------"

if [[ ${#push_functions[@]} -gt 0 ]]; then
  bash scripts/push-image.sh "${stage}" "${project}" "${short_sha}" "${push_functions[@]}"
fi

account_id="$(aws sts get-caller-identity --query Account --output text)"
region="${AWS_REGION:-$(aws configure get region || echo ap-northeast-2)}"
registry="${account_id}.dkr.ecr.${region}.amazonaws.com"
application_name="${stage}-${project}-lambda"

mkdir -p build/scripts
cp scripts/backend-deploy.sh build/scripts/backend-deploy.sh

functions_json="[]"
for fn in "${deploy_functions[@]}"; do
  image_uri="${registry}/${stage}-${project}-${fn}:${short_sha}"
  function_name="${stage}-${project}-${fn}"
  deployment_group="${stage}-${project}-${fn}-deployment-group"
  functions_json="$(jq \
    --arg name "${fn}" \
    --arg imageUri "${image_uri}" \
    --arg functionName "${function_name}" \
    --arg deploymentGroup "${deployment_group}" \
    '. + [{name: $name, imageUri: $imageUri, functionName: $functionName, deploymentGroup: $deploymentGroup}]' \
    <<<"${functions_json}")"
done

jq -n \
  --arg headCommit "${head_commit}" \
  --arg marker "${marker}" \
  --arg applicationName "${application_name}" \
  --argjson functions "${functions_json}" \
  '{headCommit: $headCommit, marker: $marker, applicationName: $applicationName, functions: $functions}' \
  > build/backend-plan.json
