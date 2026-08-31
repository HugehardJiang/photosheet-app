#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${1:?用法：generate-sha256.sh <目录> }"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${TARGET_DIR}"/* > "${TARGET_DIR}/SHA256SUMS"
else
  shasum -a 256 "${TARGET_DIR}"/* > "${TARGET_DIR}/SHA256SUMS"
fi
