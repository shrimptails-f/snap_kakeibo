#!/usr/bin/env bash
# 関数ごとに Lambda イメージを linux/arm64 でビルドして専用の ECR へ push し、
# デプロイ中タグを持つ SSM パラメータを更新する。App スタックは deploy 時にこのパラメータを解決する。
#
#   scripts/push-image.sh <stage> <project> <image tag> [function ...]
#
# 関数を省略すると backend/cmd/ 配下の全関数が対象。
# 同じタグが既に ECR にあればビルドと push は飛ばし、SSM の更新だけ行う(ECR は IMMUTABLE)。
set -euo pipefail

stage="${1:-}"
project="${2:-}"
image_tag="${3:-}"
shift 3 || true
functions=("$@")

if [[ -z "${stage}" || -z "${project}" ]]; then
  echo "usage: $0 <stage> <project> <image tag> [function ...]" >&2
  exit 1
fi
if [[ ! "${image_tag}" =~ ^[0-9a-f]{7,40}$ ]]; then
  echo "image tag must be a 7-40 character lowercase git SHA: '${image_tag}'" >&2
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required. The dev container has no Docker socket; run this from the host or CI." >&2
  exit 1
fi

if [[ ${#functions[@]} -eq 0 ]]; then
  for dir in backend/cmd/*/; do
    functions+=("$(basename "${dir}")")
  done
fi

account_id="$(aws sts get-caller-identity --query Account --output text)"
region="$(aws configure get region || true)"
region="${AWS_REGION:-${region:-ap-northeast-1}}"
registry="${account_id}.dkr.ecr.${region}.amazonaws.com"

aws ecr get-login-password --region "${region}" | docker login --username AWS --password-stdin "${registry}"
export DOCKER_BUILDKIT=1

for function_name in "${functions[@]}"; do
  if [[ ! -d "backend/cmd/${function_name}" ]]; then
    echo "backend/cmd/${function_name} does not exist" >&2
    exit 1
  fi

  repository_name="${stage}-${project}-${function_name}"
  image_uri="${registry}/${repository_name}:${image_tag}"
  parameter_name="/${stage}/${project}/functions/${function_name}/image-tag"

  if aws ecr describe-images \
    --repository-name "${repository_name}" \
    --image-ids "imageTag=${image_tag}" >/dev/null 2>&1; then
    echo "image already exists, skipping build: ${image_uri}"
  else
    docker build --platform linux/arm64 \
      --build-arg "FUNCTION_NAME=${function_name}" \
      --tag "${image_uri}" \
      --file backend/Dockerfile backend
    docker push "${image_uri}"
    echo "pushed ${image_uri}"
  fi

  aws ssm put-parameter --name "${parameter_name}" --type String --value "${image_tag}" --overwrite >/dev/null
  echo "set ${parameter_name} = ${image_tag}"
done

echo "deploy with: task infra:deploy:app"
