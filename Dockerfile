# 多阶段构建 - 构建阶段
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git

# 复制 go mod 文件并下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建参数（由 docker-compose 传入）
ARG SERVICE_NAME
ENV SERVICE_NAME=${SERVICE_NAME}

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/${SERVICE_NAME}/

# 运行阶段
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

# 从构建阶段复制二进制
COPY --from=builder /app/server /app/server
COPY --from=builder /app/configs /app/configs
COPY --from=builder /app/migrations /app/migrations

# 非 root 用户运行
RUN addgroup -g 1001 appgroup && \
    adduser -D -u 1001 -G appgroup appuser
USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/server"]
