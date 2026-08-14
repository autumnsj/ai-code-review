# ---- stage 1: 前端构建 ----
FROM node:20-alpine AS web
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- stage 2: Go 构建（纯 Go sqlite，免 CGO）----
FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN go build -trimpath -ldflags="-s -w" -o /out/aicr ./cmd/server

# ---- stage 3: 运行镜像 ----
FROM alpine:3.20
RUN apk add --no-cache git ca-certificates tini wget \
    && addgroup -S app && adduser -S -G app app \
    && mkdir -p /data && chown -R app:app /data
COPY --from=builder /out/aicr /usr/local/bin/aicr
USER app
WORKDIR /app
EXPOSE 8080
VOLUME ["/data"]
ENV AICR_DATA_DIR=/data
ENTRYPOINT ["/sbin/tini","--"]
CMD ["aicr"]
