#!/usr/bin/env bash
set -euo pipefail

APPIMAGETOOL="${APPIMAGETOOL:-appimagetool}"
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
APP_DIR="${ROOT_DIR}/dist/PhotoSheet.AppDir"
mkdir -p "${APP_DIR}/usr/bin" "${APP_DIR}/usr/share/applications"
cp "${ROOT_DIR}/dist/photosheet-linux-x64" "${APP_DIR}/usr/bin/PhotoSheet"
cp "${ROOT_DIR}/installers/linux/PhotoSheet.desktop" "${APP_DIR}/PhotoSheet.desktop"
cp "${ROOT_DIR}/installers/linux/AppRun" "${APP_DIR}/AppRun"
chmod +x "${APP_DIR}/AppRun" "${APP_DIR}/usr/bin/PhotoSheet"
"${APPIMAGETOOL}" "${APP_DIR}" "${ROOT_DIR}/dist/PhotoSheet-linux-x64.AppImage"
