#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT_DIR}/frontend"
npm ci
npm run build
rm -rf "${ROOT_DIR}/cmd/photosheet-gui/frontend/dist"
cp -R dist "${ROOT_DIR}/cmd/photosheet-gui/frontend/dist"
echo "中文 GUI 前端已同步到 Wails embed 目录"
