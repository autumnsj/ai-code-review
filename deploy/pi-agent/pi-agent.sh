#!/bin/sh
# pi-agent 启动脚本：解析软链真实路径后用 Node 运行 adapter。
# AICR_PI_AGENT_BIN 默认指向 /usr/local/bin/pi-agent（本脚本的软链）。
set -e
SOURCE=$0
while [ -L "$SOURCE" ]; do
  TARGET=$(readlink "$SOURCE")
  case $TARGET in
    /*) SOURCE=$TARGET ;;
    *)  SOURCE=$(CDPATH= cd -- "$(dirname -- "$SOURCE")" && pwd)/$TARGET ;;
  esac
done
DIR=$(CDPATH= cd -- "$(dirname -- "$SOURCE")" && pwd)
exec node "$DIR/index.mjs" "$@"
