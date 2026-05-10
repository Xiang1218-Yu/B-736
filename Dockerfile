# 构建阶段
FROM golang:1.22-alpine AS builder
WORKDIR /app

# 安装依赖
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download

# 复制源码并构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# 运行阶段
FROM scratch
WORKDIR /app

# 复制构建产物
COPY --from=builder /app/server /app/server
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
COPY --from=builder /app/locales ./locales
# 复制 CA 证书，保证 HTTPS 请求可用（如抓取外部链接）
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE 8080
CMD ["/app/server"]
