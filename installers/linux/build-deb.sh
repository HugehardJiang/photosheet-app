#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-0.1.0}"
ARCH="${ARCH:-amd64}"
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PACKAGE_ROOT="$(mktemp -d)"
trap 'rm -rf "${PACKAGE_ROOT}"' EXIT

mkdir -p "${PACKAGE_ROOT}/DEBIAN" "${PACKAGE_ROOT}/usr/bin" "${PACKAGE_ROOT}/usr/share/applications"
cp "${ROOT_DIR}/dist/photosheet-linux-x64" "${PACKAGE_ROOT}/usr/bin/PhotoSheet"
cp "${ROOT_DIR}/installers/linux/PhotoSheet.desktop" "${PACKAGE_ROOT}/usr/share/applications/photosheet.desktop"
cat > "${PACKAGE_ROOT}/DEBIAN/control" <<EOF
Package: photosheet
Version: ${VERSION}
Section: graphics
Priority: optional
Architecture: ${ARCH}
Maintainer: PhotoSheet contributors
Description: 中文照片拼接与拆分工具
EOF
dpkg-deb --build "${PACKAGE_ROOT}" "${ROOT_DIR}/dist/photosheet_${VERSION}_${ARCH}.deb"
