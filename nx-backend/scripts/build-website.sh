#!/bin/sh
# build-website.sh — 构建 website-react 并原子发布到 nginx 静态目录。
#
# 由 Go server 在保存官网配置后自动调用（见 internal/siteconfig/builder.go）。
# 也可手动执行：sh scripts/build-website.sh
#
# 可通过环境变量覆盖（均有默认值，按本仓库布局推断）：
#   WEBSITE_DIR          website-react 源码目录
#   WEBSITE_PUBLISH_DIR  发布目标目录（nginx root）
#   NODE_BIN / NPM_BIN   指定 node/npm 可执行文件（cron/服务场景 PATH 可能不全时用）
set -eu

# 脚本所在目录（nx-backend/scripts）的上两级即仓库根 nine-xing。
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

WEBSITE_DIR="${WEBSITE_DIR:-$REPO_ROOT/website-react}"
WEBSITE_PUBLISH_DIR="${WEBSITE_PUBLISH_DIR:-/usr/share/nginx/html}"
NPM_BIN="${NPM_BIN:-npm}"

echo "[build-website] repo root      : $REPO_ROOT"
echo "[build-website] website dir    : $WEBSITE_DIR"
echo "[build-website] publish dir    : $WEBSITE_PUBLISH_DIR"
echo "[build-website] npm            : $NPM_BIN"

if [ ! -d "$WEBSITE_DIR" ]; then
  echo "[build-website] ERROR: WEBSITE_DIR 不存在: $WEBSITE_DIR" >&2
  exit 1
fi

cd "$WEBSITE_DIR"

# 安装依赖：有 lock 用 ci，否则 install；node_modules 已存在则跳过以加速。
if [ ! -d node_modules ]; then
  if [ -f package-lock.json ]; then
    echo "[build-website] installing deps (npm ci)…"
    "$NPM_BIN" ci
  else
    echo "[build-website] installing deps (npm install)…"
    "$NPM_BIN" install
  fi
fi

echo "[build-website] building…"
"$NPM_BIN" run build

if [ ! -d "$WEBSITE_DIR/dist" ]; then
  echo "[build-website] ERROR: 构建未产出 dist/" >&2
  exit 1
fi

# 原子发布：先复制到同分区的临时目录，再 mv 覆盖，避免发布过程中半成品被访问。
mkdir -p "$WEBSITE_PUBLISH_DIR"
PARENT_DIR=$(dirname "$WEBSITE_PUBLISH_DIR")
TMP_DIR="$PARENT_DIR/.site-publish.tmp.$$"
OLD_DIR="$PARENT_DIR/.site-publish.old.$$"

rm -rf "$TMP_DIR" "$OLD_DIR"
mkdir -p "$TMP_DIR"
cp -R "$WEBSITE_DIR/dist/." "$TMP_DIR/"

# 把现有内容换出、新内容换入，尽量缩小不可用窗口。
if [ -d "$WEBSITE_PUBLISH_DIR" ]; then
  mv "$WEBSITE_PUBLISH_DIR" "$OLD_DIR"
fi
mv "$TMP_DIR" "$WEBSITE_PUBLISH_DIR"
rm -rf "$OLD_DIR"

echo "[build-website] done. published to $WEBSITE_PUBLISH_DIR"
