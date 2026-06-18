#!/bin/sh
# 首启播种：若持久化卷里还没有站点配置，用镜像内置的默认值初始化。
# 之后后台保存写入的就是这份持久化文件，重启不丢。
set -eu

CONFIG_PATH="${SITE_CONFIG_PATH:-/data/site-config.json}"

if [ ! -f "$CONFIG_PATH" ]; then
  echo "[entrypoint] 初始化站点配置 -> $CONFIG_PATH"
  mkdir -p "$(dirname "$CONFIG_PATH")"
  cp /app/default-site-config.json "$CONFIG_PATH"
fi

exec "$@"
