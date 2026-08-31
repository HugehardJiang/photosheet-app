#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH="${1:-$(uname -m)}"
WAILS_VERSION="${WAILS_VERSION:-v2.10.2}"
MACOS_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-11.0}"

case "${ARCH}" in
  arm64|amd64) ;;
  *) echo "用法：$0 [arm64|amd64]" >&2; exit 2 ;;
esac

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS GUI 必须在 Darwin 上构建" >&2
  exit 1
fi

NATIVE_ARTIFACT="${ROOT_DIR}/internal/raw/native/darwin/${ARCH}/libphotosheet_raw.dylib"
NATIVE_TAGS="wails"
if [[ "${PHOTOSHEET_NATIVE_MACOS:-0}" == "1" ]]; then
  if [[ ! -f "${NATIVE_ARTIFACT}" ]]; then
    echo "缺少 macOS ${ARCH} 原生 RAW artifact：${NATIVE_ARTIFACT}" >&2
    echo "先运行 scripts/build-native-macos.sh --arch ${ARCH}" >&2
    exit 1
  fi
  NATIVE_TAGS="wails photosheet_native"
else
  echo "构建 core GUI：未启用本地 native RAW backend（设置 PHOTOSHEET_NATIVE_MACOS=1 可启用）"
fi

"${ROOT_DIR}/scripts/build-gui.sh"

cd "${ROOT_DIR}/cmd/photosheet-gui"
if command -v wails >/dev/null 2>&1; then
  WAILS=(wails)
else
  WAILS=(go run "github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}")
fi

MACOSX_DEPLOYMENT_TARGET="${MACOS_DEPLOYMENT_TARGET}" \
CGO_CFLAGS="-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}" \
CGO_CXXFLAGS="-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}" \
CGO_LDFLAGS="-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}" \
CGO_ENABLED=1 "${WAILS[@]}" build \
  -clean \
  -platform "darwin/${ARCH}" \
  -tags "${NATIVE_TAGS}" \
  -trimpath \
  -s

APP_PATH="${ROOT_DIR}/cmd/photosheet-gui/build/bin/PhotoSheet.app"
if [[ -d "${APP_PATH}" ]]; then
  echo "macOS GUI 已生成：${APP_PATH}"
  /usr/bin/file "${APP_PATH}/Contents/MacOS/PhotoSheet"
  /usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "${APP_PATH}/Contents/Info.plist"
else
  echo "Wails 未生成预期 app bundle：${APP_PATH}" >&2
  exit 1
fi
