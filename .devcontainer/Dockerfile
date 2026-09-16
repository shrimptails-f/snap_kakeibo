FROM golang:1.26-alpine3.22

ARG TASK_VERSION=v3.45.4
ENV TZ=Asia/Tokyo \
    GOPATH=/home/dev/go \
    NPM_CONFIG_PREFIX=/home/dev/.local \
    USE_BUILTIN_RIPGREP=0 \
    PATH=/home/dev/.local/bin:/home/dev/go/bin:${PATH}

RUN apk add --no-cache \
      aws-cli bash build-base ca-certificates curl git libgcc libstdc++ \
      nodejs npm ripgrep tar tzdata && \
    cp /usr/share/zoneinfo/${TZ} /etc/localtime && \
    echo "${TZ}" > /etc/timezone && \
    arch="$(apk --print-arch)" && \
    case "${arch}" in \
      x86_64) task_arch=amd64 ;; \
      aarch64) task_arch=arm64 ;; \
      *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/go-task/task/releases/download/${TASK_VERSION}/task_linux_${task_arch}.tar.gz" \
      | tar -xz -C /usr/local/bin task && \
    addgroup -g 1000 dev && \
    adduser -D -u 1000 -G dev -s /bin/bash dev && \
    mkdir -p /workspace/snap_kakeibo /home/dev/.local /home/dev/.codex \
      /home/dev/.claude /home/dev/go/pkg/mod && \
    chown -R dev:dev /workspace /home/dev

COPY --chown=dev:dev .devcontainer/.bashrc /home/dev/.bashrc

WORKDIR /workspace/snap_kakeibo
USER dev

RUN npm install --global @openai/codex@latest aws-cdk@2.1141.0 && \
    curl -fsSL https://claude.ai/install.sh -o /tmp/install-claude.sh && \
    bash /tmp/install-claude.sh && \
    rm /tmp/install-claude.sh && \
    task --version && codex --version && claude --version && cdk --version
