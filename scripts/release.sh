#!/usr/bin/env bash
# 一键发布脚本：本地构建前端 + 交叉编译 Go 二进制（已内嵌前端），上传到目标机，
# 在目标机构建 Docker 镜像并用 docker compose 重启，最后轮询健康状态。
#
# 这是开源项目，所有与环境相关的值都通过环境变量注入，不写死任何主机/目录/镜像名。
# 脚本只更新镜像与构建上下文，不上传也不覆盖目标机上的 docker-compose.yml / aicr.json
# （这些含数据卷与密钥配置，由目标机自行维护）。
#
# 用法：
#   DEPLOY_HOST=user@your-host ./scripts/release.sh
#
# 可选环境变量见下方默认值。
set -euo pipefail

# ---------- 可配置项（环境变量覆盖） ----------
DEPLOY_HOST="${DEPLOY_HOST:?请设置 DEPLOY_HOST，例如 user@your-host 或 SSH 别名}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/aicr/build-ctx}"   # 远程构建上下文目录
COMPOSE_DIR="${COMPOSE_DIR:-/opt/aicr}"           # 远程 docker-compose.yml 所在目录
IMAGE="${IMAGE:-aicr:latest}"                     # 构建出的镜像 tag
CONTAINER="${CONTAINER:-aicr}"                    # 健康轮询的容器名
GOARCH_VAL="${GOARCH:-amd64}"                     # 交叉编译目标架构
GOOS_VAL="${GOOS:-linux}"
BINARY_NAME="aicr-linux-${GOARCH_VAL}"

# ---------- 工具函数 ----------
log()  { printf '\033[1;36m[release]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[release]\033[0m %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "未找到命令 $1，请先安装"
}

# 切换到仓库根目录（脚本位于 scripts/ 下）。
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# macOS 打包时避免产生 ._ 资源分叉文件。
export COPYFILE_DISABLE=1

# 发布前确认没有未提交的本地改动不是必须的；只做工具检查。
require_cmd ssh
require_cmd go

# ---------- 1. 前端构建 ----------
if [ -n "${SKIP_WEB:-}" ]; then
  log "SKIP_WEB 已设置，跳过前端构建（注意：二进制内嵌的是现有 web/dist）"
else
  require_cmd npm
  log "构建前端 (web/) ..."
  ( cd web && npm ci && npm run build )
fi

# ---------- 2. Go 交叉编译 ----------
if [ -n "${SKIP_GO:-}" ]; then
  log "SKIP_GO 已设置，跳过 Go 构建（复用已有 ${BINARY_NAME}）"
  [ -f "$BINARY_NAME" ] || die "SKIP_GO 已设置但找不到 $BINARY_NAME"
else
  log "交叉编译 $BINARY_NAME (GOOS=$GOOS_VAL GOARCH=$GOARCH_VAL) ..."
  CGO_ENABLED=0 GOOS="$GOOS_VAL" GOARCH="$GOARCH_VAL" \
    go build -trimpath -ldflags="-s -w" -o "$BINARY_NAME" ./cmd/server
fi

# ---------- 3. 上传构建上下文 ----------
# 需要上传：二进制、Dockerfile.release、deploy/pi-agent/。
log "上传构建上下文到 $DEPLOY_HOST:$DEPLOY_DIR ..."
ssh "$DEPLOY_HOST" "mkdir -p '$DEPLOY_DIR/deploy'"

upload() {
  # $1 = 本地路径, $2 = 远程相对路径。
  # 目录必须把「内容」同步到目标目录本身，而不是把目录嵌套进去：
  # rsync 源/目标都带尾斜杠表示同步内容；scp 先清空目标再整体复制。
  if [ -d "$1" ]; then
    if command -v rsync >/dev/null 2>&1 && ssh "$DEPLOY_HOST" 'command -v rsync >/dev/null 2>&1'; then
      rsync -az --delete -e ssh "${1%/}/" "$DEPLOY_HOST:$DEPLOY_DIR/${2%/}/"
    else
      ssh "$DEPLOY_HOST" "rm -rf '$DEPLOY_DIR/$2' && mkdir -p '$DEPLOY_DIR/$2'"
      scp -r "$1/." "$DEPLOY_HOST:$DEPLOY_DIR/$2/"
    fi
  else
    if command -v rsync >/dev/null 2>&1 && ssh "$DEPLOY_HOST" 'command -v rsync >/dev/null 2>&1'; then
      rsync -az -e ssh "$1" "$DEPLOY_HOST:$DEPLOY_DIR/$2"
    else
      scp "$1" "$DEPLOY_HOST:$DEPLOY_DIR/$2"
    fi
  fi
}

upload "$BINARY_NAME"        "$BINARY_NAME"
upload "Dockerfile.release"  "Dockerfile.release"
upload "deploy/pi-agent"     "deploy/pi-agent"

# ---------- 4. 远程构建镜像 ----------
log "在目标机构建镜像 $IMAGE ..."
ssh "$DEPLOY_HOST" "cd '$DEPLOY_DIR' && docker build -f Dockerfile.release -t '$IMAGE' ."

# ---------- 5. 远程重启 ----------
log "通过 docker compose 重启服务 ($COMPOSE_DIR) ..."
ssh "$DEPLOY_HOST" "cd '$COMPOSE_DIR' && docker compose up -d"

# ---------- 6. 健康轮询 ----------
log "等待容器 $CONTAINER 健康 ..."
HEALTHY=""
for i in $(seq 1 30); do
  status="$(ssh "$DEPLOY_HOST" "docker inspect -f '{{.State.Health.Status}}' '$CONTAINER' 2>/dev/null" || true)"
  if [ "$status" = "healthy" ]; then
    HEALTHY=1
    break
  fi
  printf '  ... %s (%d/30)\n' "${status:-starting}" "$i"
  sleep 2
done

if [ -z "$HEALTHY" ]; then
  err "容器在 60s 内未进入 healthy 状态，打印最后 50 行日志："
  ssh "$DEPLOY_HOST" "docker logs --tail 50 '$CONTAINER'" || true
  die "部署失败，请检查容器日志"
fi

IMAGE_SHA="$(ssh "$DEPLOY_HOST" "docker inspect -f '{{.Image}}' '$CONTAINER' 2>/dev/null" || true)"
log "部署完成 ✓ 容器 $CONTAINER 已健康运行${IMAGE_SHA:+（镜像 ${IMAGE_SHA:0:19}）}"
