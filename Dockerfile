# 多阶段构建：编译 → 极简运行镜像
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 构建信息通过 ARG 注入，便于状态页展示版本/提交/构建时间
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w \
      -X campus-service-platform/internal/pkg/buildinfo.Version=${VERSION} \
      -X campus-service-platform/internal/pkg/buildinfo.Commit=${COMMIT} \
      -X campus-service-platform/internal/pkg/buildinfo.Time=${BUILD_TIME}" \
    -o /out/campus-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=builder /out/campus-server /app/campus-server
COPY --from=builder /src/config.yaml /app/config.yaml
ENV SERVER_PORT=8081 UPLOAD_DIR=/data/uploads
EXPOSE 8081
ENTRYPOINT ["/app/campus-server"]
