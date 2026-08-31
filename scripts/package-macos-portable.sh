#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH="${1:-$(uname -m)}"

case "${ARCH}" in
  arm64|amd64) ;;
  *)
    echo "用法：$0 [arm64|amd64]" >&2
    exit 2
    ;;
esac

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS portable app 必须在 Darwin 上构建" >&2
  exit 1
fi

"${ROOT_DIR}/scripts/build-gui-macos.sh" "${ARCH}"

SOURCE_APP="${ROOT_DIR}/cmd/photosheet-gui/build/bin/PhotoSheet.app"
OUTPUT_DIR="${ROOT_DIR}/dist"
OUTPUT_APP="${OUTPUT_DIR}/PhotoSheet-macos-${ARCH}-portable.app"
OUTPUT_ZIP="${OUTPUT_DIR}/PhotoSheet-macos-${ARCH}-portable.zip"

if [[ ! -d "${SOURCE_APP}" ]]; then
  echo "缺少 Wails app bundle：${SOURCE_APP}" >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"
if [[ -e "${OUTPUT_APP}" ]]; then
  rm -rf "${OUTPUT_APP}"
fi
if [[ -e "${OUTPUT_ZIP}" ]]; then
  rm -f "${OUTPUT_ZIP}"
fi

ditto "${SOURCE_APP}" "${OUTPUT_APP}"

# 本机 ad-hoc 签名，方便 Finder 启动；这不是 Developer ID 签名或 notarization。
codesign --force --deep --sign - "${OUTPUT_APP}"
codesign --verify --deep --strict "${OUTPUT_APP}"

# 保持 app bundle 目录结构，解压后可直接双击 PhotoSheet.app。
ditto -c -k --sequesterRsrc --keepParent "${OUTPUT_APP}" "${OUTPUT_ZIP}"

echo "macOS 便携版已生成："
/usr/bin/file "${OUTPUT_APP}/Contents/MacOS/PhotoSheet"
/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "${OUTPUT_APP}/Contents/Info.plist"
du -sh "${OUTPUT_APP}" "${OUTPUT_ZIP}"
shasum -a 256 "${OUTPUT_ZIP}"
echo "App：${OUTPUT_APP}"
echo "ZIP：${OUTPUT_ZIP}"
