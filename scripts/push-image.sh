#!/usr/bin/env bash
# 関数ごとに Lambda イメージを linux/arm64 でビルドして専用の ECR へ push し、
# デプロイ中タグを持つ SSM パラメータを更新する。App スタックは deploy 時にこのパラメータを解決する。
#
#   scripts/push-image.sh <stage> <project> [image tag|-] [function ...]
#
# image tag を省略するか "-" にすると git rev-parse --short HEAD を使う。
# 関数を省略すると backend/cmd/ 配下の全関数が対象。
# 同じタグが既に ECR にあればビルドと push は飛ばし、SSM の更新だけ行う(ECR は IMMUTABLE)。
set -euo pipefail

stage="${1:-}"
project="${2:-}"
image_tag="${3:-}"
shift "$(( $# < 3 ? $# : 3 ))"
functions=("$@")

if [[ -z "${image_tag}" || "${image_tag}" == "-" ]]; then
  image_tag="$(git rev-parse --short HEAD)"
fi

if [[ -z "${stage}" || -z "${project}" ]]; then
  echo "usage: $0 <stage> <project> <image tag> [function ...]" >&2
  exit 1
fi
if [[ ! "${image_tag}" =~ ^[0-9a-f]{7,40}$ ]]; then
  echo "image tag must be a 7-40 character lowercase git SHA: '${image_tag}'" >&2
  exit 1
fi
if [[ ${#functions[@]} -eq 0 ]]; then
  for dir in backend/cmd/*/; do
    functions+=("$(basename "${dir}")")
  done
fi
for function_name in "${functions[@]}"; do
  if [[ ! -d "backend/cmd/${function_name}" ]]; then
    echo "backend/cmd/${function_name} does not exist" >&2
    exit 1
  fi
done

account_id="$(aws sts get-caller-identity --query Account --output text)"
region="$(aws configure get region || true)"
region="${AWS_REGION:-${region:-ap-northeast-1}}"
registry="${account_id}.dkr.ecr.${region}.amazonaws.com"

repository_name() { echo "${stage}-${project}-$1"; }
image_uri() { echo "${registry}/$(repository_name "$1"):${image_tag}"; }

# 同じタグが既に ECR にある関数はビルドしない(IMMUTABLE なので push もできない)
to_build=()
for function_name in "${functions[@]}"; do
  if aws ecr describe-images \
    --repository-name "$(repository_name "${function_name}")" \
    --image-ids "imageTag=${image_tag}" >/dev/null 2>&1; then
    echo "image already exists, skipping build: $(image_uri "${function_name}")"
  else
    to_build+=("${function_name}")
  fi
done

if [[ ${#to_build[@]} -gt 0 ]]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required. The dev container has no Docker socket; run this from the host or CI." >&2
    exit 1
  fi
  export DOCKER_BUILDKIT=1

  aws ecr get-login-password --region "${region}" | docker login --username AWS --password-stdin "${registry}"

  # builder ステージ(依存の取得と全パッケージのビルド)を先に1回だけ通してキャッシュを温める。
  # 関数ごとのビルドはこのレイヤーとキャッシュマウントを土台にするので、差分のリンクだけで済む
  docker build --platform linux/arm64 \
    --target builder \
    --tag "${project}-builder:${image_tag}" \
    --file backend/Dockerfile backend

  for function_name in "${to_build[@]}"; do
    uri="$(image_uri "${function_name}")"
    docker build --platform linux/arm64 \
      --build-arg "FUNCTION_NAME=${function_name}" \
      --tag "${uri}" \
      --file backend/Dockerfile backend
    docker push "${uri}"
    echo "pushed ${uri}"
  done
fi

for function_name in "${functions[@]}"; do
  parameter_name="/${stage}/${project}/functions/${function_name}/image-tag"
  aws ssm put-parameter --name "${parameter_name}" --type String --value "${image_tag}" --overwrite >/dev/null
  echo "set ${parameter_name} = ${image_tag}"
done

echo "deploy with: task infra:deploy:app"
