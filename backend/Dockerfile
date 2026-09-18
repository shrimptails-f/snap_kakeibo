# syntax=docker/dockerfile:1
# Lambda 用イメージ。関数ごとに build-arg FUNCTION_NAME を変えてビルドし、専用の ECR へ push する。
#
# builder ステージは関数に依存しないので、全関数のビルドで同じレイヤーが使い回される。
# Go の module キャッシュとビルドキャッシュは cache mount で Docker ビルド間で共有し、
# builder で全パッケージを一度ビルドして温めておくと、関数ごとのビルドはリンクだけになる。
FROM --platform=${BUILDPLATFORM} golang:1.26-alpine AS builder

ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}
WORKDIR /src

COPY go.mod go.sum* ./
RUN --mount=type=cache,id=snap-kakeibo-gomod,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,id=snap-kakeibo-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=snap-kakeibo-gobuild,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" ./...

FROM builder AS function

ARG FUNCTION_NAME
RUN --mount=type=cache,id=snap-kakeibo-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=snap-kakeibo-gobuild,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /bootstrap "./cmd/${FUNCTION_NAME}"

FROM public.ecr.aws/lambda/provided:al2023

COPY --from=function /bootstrap /var/runtime/bootstrap

ENTRYPOINT ["/var/runtime/bootstrap"]
