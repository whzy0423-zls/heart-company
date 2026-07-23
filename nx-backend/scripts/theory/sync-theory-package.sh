#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
DEFAULT_PACKAGE="$REPO_ROOT/data/theory/xinzhili/round-001"

if [ "$#" -eq 0 ]; then
  echo "用法: $0 <validate|plan|stage|review|promote|activate> [参数]" >&2
  exit 2
fi

command_name=$1
shift

has_package=false
for argument in "$@"; do
  if [ "$argument" = "--package" ] || [ "${argument#--package=}" != "$argument" ]; then
    has_package=true
  fi
done

if [ -n "${THEORYSYNC_BIN:-}" ]; then
  runner=$THEORYSYNC_BIN
  if [ ! -x "$runner" ]; then
    echo "THEORYSYNC_BIN 不是可执行文件" >&2
    exit 2
  fi
  if { [ "$command_name" = "validate" ] || [ "$command_name" = "plan" ] || [ "$command_name" = "stage" ]; } && [ "$has_package" = false ]; then
    exec "$runner" "$command_name" --package "$DEFAULT_PACKAGE" "$@"
  fi
  exec "$runner" "$command_name" "$@"
fi

cd "$REPO_ROOT/nx-backend/apps/server"
if { [ "$command_name" = "validate" ] || [ "$command_name" = "plan" ] || [ "$command_name" = "stage" ]; } && [ "$has_package" = false ]; then
  exec go run ./cmd/theorysync "$command_name" --package "$DEFAULT_PACKAGE" "$@"
fi
exec go run ./cmd/theorysync "$command_name" "$@"
