#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
GO_BIN="${GO_BIN:-go}"
VERSION="${VERSION:-0.1.0}"
NATIVE_MACOS="${PHOTOSHEET_NATIVE_MACOS:-0}"
NATIVE_WINDOWS="${PHOTOSHEET_NATIVE_WINDOWS:-0}"
MACOS_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-11.0}"

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

build_one() {
  local goos="$1"
  local goarch="$2"
  local suffix="$3"
  local ext=""
  local cgo_enabled=0
  local tag_args=()
  local go_env=("GOOS=${goos}" "GOARCH=${goarch}" "CGO_ENABLED=${cgo_enabled}")
  if [[ "${goos}" == "windows" ]]; then ext=".exe"; fi
  if [[ "${goos}" == "windows" && "${goarch}" == "amd64" && "${NATIVE_WINDOWS}" == "1" ]]; then
    local native_artifact="${ROOT_DIR}/internal/raw/native/windows/libraw.dll"
    if [[ ! -f "${native_artifact}" ]]; then
      echo "missing Windows x64 native RAW backend: ${native_artifact}" >&2
      echo "place a reviewed local artifact there or set PHOTOSHEET_NATIVE_WINDOWS=0" >&2
      exit 1
    fi
    tag_args=(-tags photosheet_native)
  fi

  if [[ "${goos}" == "darwin" && ("${goarch}" == "arm64" || "${goarch}" == "amd64") && "${NATIVE_MACOS}" == "1" ]]; then
    local native_artifact="${ROOT_DIR}/internal/raw/native/darwin/${goarch}/libphotosheet_raw.dylib"
    if [[ ! -f "${native_artifact}" ]]; then
      echo "missing macOS ${goarch} native RAW backend: ${native_artifact}" >&2
      echo "run scripts/build-native-macos.sh --arch ${goarch} first" >&2
      exit 1
    fi
    cgo_enabled=1
    tag_args=(-tags photosheet_native)
    go_env=("GOOS=${goos}" "GOARCH=${goarch}" "CGO_ENABLED=${cgo_enabled}")
  fi

  if [[ "${goos}" == "darwin" ]]; then
    go_env+=("MACOSX_DEPLOYMENT_TARGET=${MACOS_DEPLOYMENT_TARGET}")
    if [[ "${cgo_enabled}" == "1" ]]; then
      go_env+=("CGO_CFLAGS=-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}")
      go_env+=("CGO_CXXFLAGS=-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}")
      go_env+=("CGO_LDFLAGS=-mmacosx-version-min=${MACOS_DEPLOYMENT_TARGET}")
    fi
  fi

  if ((${#tag_args[@]} > 0)); then
    env "${go_env[@]}" "${GO_BIN}" build "${tag_args[@]}" -trimpath -ldflags "-s -w -X photosheet/internal/project.AppVersion=${VERSION}" -o "${DIST_DIR}/photosheet-${suffix}${ext}" ./cmd/photosheet
  else
    env "${go_env[@]}" "${GO_BIN}" build -trimpath -ldflags "-s -w -X photosheet/internal/project.AppVersion=${VERSION}" -o "${DIST_DIR}/photosheet-${suffix}${ext}" ./cmd/photosheet
  fi
  local updater_env=("GOOS=${goos}" "GOARCH=${goarch}" "CGO_ENABLED=0")
  if [[ "${goos}" == "darwin" ]]; then
    updater_env+=("MACOSX_DEPLOYMENT_TARGET=${MACOS_DEPLOYMENT_TARGET}")
  fi
  env "${updater_env[@]}" "${GO_BIN}" build -trimpath -ldflags "-s -w" -o "${DIST_DIR}/photosheet-updater-${suffix}${ext}" ./cmd/photosheet-updater
}

build_one darwin amd64 macos-intel
build_one darwin arm64 macos-arm64
build_one linux amd64 linux-x64
build_one linux arm64 linux-arm64
build_one windows amd64 windows-x64
build_one windows arm64 windows-arm64

(
  cd "${DIST_DIR}"
  : > SHA256SUMS.tmp
  for artifact in *; do
    if [[ "${artifact}" == "SHA256SUMS" || "${artifact}" == "SHA256SUMS.tmp" || ! -f "${artifact}" ]]; then
      continue
    fi
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "${artifact}" >> SHA256SUMS.tmp
    else
      shasum -a 256 "${artifact}" >> SHA256SUMS.tmp
    fi
  done
  sort SHA256SUMS.tmp > SHA256SUMS
  rm -f SHA256SUMS.tmp
)
echo "已生成发布 CLI/updater artifact：${DIST_DIR}"
