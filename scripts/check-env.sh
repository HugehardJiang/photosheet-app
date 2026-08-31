#!/usr/bin/env bash
set -euo pipefail

echo "PhotoSheet build environment"

for command_name in git node npm go; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少必需工具：$command_name" >&2
    exit 1
  fi
  if [[ "$command_name" == "go" ]]; then
    "$command_name" version
  else
    "$command_name" --version
  fi
done

echo
echo "核心构建默认使用纯 Go RAW fallback。"
echo "原生 RAW 构建是可选项，需要按 native/README.md 准备并审查 SDK。"
