#!/usr/bin/env bash
set -euo pipefail

APP_PATH="${1:?用法：build-dmg.sh <PhotoSheet.app> }"
OUTPUT="${2:-PhotoSheet.dmg}"

if [[ -n "${CODESIGN_IDENTITY:-}" ]]; then
  codesign --deep --force --options runtime --sign "${CODESIGN_IDENTITY}" "${APP_PATH}"
fi
STAGING="$(mktemp -d)"
trap 'rm -rf "${STAGING}"' EXIT
cp -R "${APP_PATH}" "${STAGING}/PhotoSheet.app"
hdiutil create -volname "PhotoSheet" -srcfolder "${STAGING}" -ov -format UDZO "${OUTPUT}"

if [[ -n "${NOTARIZE_PROFILE:-}" ]]; then
  xcrun notarytool submit "${OUTPUT}" --keychain-profile "${NOTARIZE_PROFILE}" --wait
  xcrun stapler staple "${OUTPUT}"
fi
