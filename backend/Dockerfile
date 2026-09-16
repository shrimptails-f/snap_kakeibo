# Lambda 用イメージ。関数ごとに build-arg FUNCTION_NAME を変えてビルドし、専用の ECR へ push する。
FROM --platform=${BUILDPLATFORM} golang:1.26-alpine AS builder

ARG FUNCTION_NAME
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w" -o /bootstrap "./cmd/${FUNCTION_NAME}"

FROM public.ecr.aws/lambda/provided:al2023

COPY --from=builder /bootstrap /var/runtime/bootstrap

ENTRYPOINT ["/var/runtime/bootstrap"]
