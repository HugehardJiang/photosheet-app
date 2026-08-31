#!/usr/bin/env bash
set -euo pipefail

# Build the self-contained macOS RAW backend from the upstream source archives
# named in docs/MACOS_HANDOFF.md. The output is intentionally a per-arch
# artifact so a universal release can be assembled without mixing libraries.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH="$(uname -m)"
WORK_ROOT="${PHOTOSHEET_NATIVE_WORK_ROOT:-${TMPDIR:-/tmp}/photosheet-native}"
OUTPUT_ROOT="${PHOTOSHEET_NATIVE_OUTPUT_ROOT:-${ROOT_DIR}/internal/raw/native/darwin}"
DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-11.0}"
JOBS="${JOBS:-$(sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

LIBRAW_URL="https://www.libraw.org/data/LibRaw-0.22.2.zip"
LIBRAW_SHA256="f4448371523f9960d26a131ed9dc21bee49f967adbcc4fb6f56b9b47c2ba87c6"
DNG_SHA256="73499b47f4683e12120a234bd0946f02e52ab2ff9834bcbd0e9f8ab4f923360e"

usage() {
  echo "用法：$0 [--arch arm64|amd64|both] [--work-root DIR] [--output-root DIR]"
  echo
  echo "默认只构建当前机器架构，并把 dylib 安装到 internal/raw/native/darwin/<arch>/。"
}

while (($# > 0)); do
  case "$1" in
    --arch)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ARCH="$2"
      shift 2
      ;;
    --work-root)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      WORK_ROOT="$2"
      shift 2
      ;;
    --output-root)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      OUTPUT_ROOT="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "${ARCH}" in
  arm64|amd64|both) ;;
  *) echo "不支持的 macOS 架构：${ARCH}" >&2; exit 2 ;;
esac

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS 原生 RAW 后端必须在 Darwin 上构建" >&2
  exit 1
fi

for command_name in cmake ninja clang clang++ make unzip curl shasum otool; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "缺少构建工具：${command_name}" >&2
    exit 1
  fi
done

CMAKE_BIN="$(command -v cmake)"
NINJA_BIN="$(command -v ninja)"
CC_BIN="${CC:-$(command -v clang)}"
CXX_BIN="${CXX:-$(command -v clang++)}"

mkdir -p "${WORK_ROOT}/downloads" "${WORK_ROOT}/sources" "${OUTPUT_ROOT}"

download_verified() {
  local url="$1"
  local destination="$2"
  local expected="$3"
  if [[ ! -f "${destination}" ]]; then
    echo "下载 ${url}"
    curl -fL --retry 3 --retry-all-errors -o "${destination}.part" "${url}"
    mv "${destination}.part" "${destination}"
  fi
  local actual
  actual="$(shasum -a 256 "${destination}" | awk '{print $1}')"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "SHA-256 校验失败：${destination}" >&2
    echo "期望：${expected}" >&2
    echo "实际：${actual}" >&2
    exit 1
  fi
}

verify_existing() {
  local destination="$1"
  local expected="$2"
  if [[ ! -f "${destination}" ]]; then
    echo "找不到本地归档：${destination}" >&2
    exit 1
  fi
  local actual
  actual="$(shasum -a 256 "${destination}" | awk '{print $1}')"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "SHA-256 校验失败：${destination}" >&2
    echo "期望：${expected}" >&2
    echo "实际：${actual}" >&2
    exit 1
  fi
}

extract_verified() {
  local archive="$1"
  local destination="$2"
  local expected_directory="$3"
  if [[ ! -d "${destination}/${expected_directory}" ]]; then
    rm -rf "${destination}"
    mkdir -p "${destination}"
    unzip -q "${archive}" -d "${destination}"
  fi
  if [[ ! -d "${destination}/${expected_directory}" ]]; then
    echo "归档目录结构不符合预期：${archive}" >&2
    exit 1
  fi
}

LIBRAW_ARCHIVE="${WORK_ROOT}/downloads/LibRaw-0.22.2.zip"
DNG_ARCHIVE="${PHOTOSHEET_NATIVE_DNG_SDK_ARCHIVE:-}"
download_verified "${LIBRAW_URL}" "${LIBRAW_ARCHIVE}" "${LIBRAW_SHA256}"
if [[ -z "${DNG_ARCHIVE}" ]]; then
  echo "不会自动下载 Adobe DNG SDK。请从 Adobe 官方 DNG SDK 页面获取归档，审查其许可后设置：" >&2
  echo "PHOTOSHEET_NATIVE_DNG_SDK_ARCHIVE=/absolute/path/to/reviewed-dng-sdk.zip" >&2
  echo "参考页面：https://helpx.adobe.com/camera-raw/digital-negative.html" >&2
  exit 1
fi
verify_existing "${DNG_ARCHIVE}" "${DNG_SHA256}"
extract_verified "${LIBRAW_ARCHIVE}" "${WORK_ROOT}/sources/libraw" "LibRaw-0.22.2"
extract_verified "${DNG_ARCHIVE}" "${WORK_ROOT}/sources/dng" "dng_sdk_1_7_1"

LIBRAW_ROOT="${WORK_ROOT}/sources/libraw/LibRaw-0.22.2"
DNG_SDK_ROOT="${WORK_ROOT}/sources/dng/dng_sdk_1_7_1"
DNG_ROOT="${DNG_SDK_ROOT}/dng_sdk/source"
XMP_ROOT="${DNG_SDK_ROOT}/xmp/toolkit"
JXL_SOURCE_ROOT="${DNG_SDK_ROOT}/libjxl/libjxl"
JPEG_SOURCE_ROOT="${DNG_SDK_ROOT}/libjpeg"

build_one() {
  local arch="$1"
  local arch_root="${WORK_ROOT}/${arch}"
  local jpeg_source="${arch_root}/libjpeg-source"
  local jpeg_build="${arch_root}/libjpeg-build"
  local jpeg_prefix="${arch_root}/libjpeg-prefix"
  local jxl_build="${arch_root}/libjxl-build"
  local native_build="${arch_root}/native-build"
  local native_stage="${arch_root}/native-stage"
  local destination="${OUTPUT_ROOT}/${arch}/libphotosheet_raw.dylib"
  local target_arch="${arch}"
  local jpeg_configure_host=()
  if [[ "${arch}" == "amd64" ]]; then target_arch="x86_64"; fi
  if [[ "${arch}" == "amd64" ]]; then jpeg_configure_host=(--host=x86_64-apple-darwin); fi
  local arch_flags="-arch ${target_arch} -mmacosx-version-min=${DEPLOYMENT_TARGET}"

  echo "==> 构建 macOS ${arch} RAW backend"
  rm -rf "${arch_root}"
  mkdir -p "${arch_root}" "${native_stage}" "$(dirname "${destination}")"

  cp -R "${JPEG_SOURCE_ROOT}" "${jpeg_source}"
  (
    cd "${jpeg_source}"
    CC="${CC_BIN}" \
    CFLAGS="-O2 -fPIC ${arch_flags}" \
    CPPFLAGS="${arch_flags}" \
    LDFLAGS="${arch_flags}" \
      ./configure "${jpeg_configure_host[@]}" \
        --disable-shared --enable-static --prefix="${jpeg_prefix}"
    make -j"${JOBS}"
    make install
  )

  "${CMAKE_BIN}" -S "${JXL_SOURCE_ROOT}" -B "${jxl_build}" -G Ninja \
    -DCMAKE_MAKE_PROGRAM="${NINJA_BIN}" \
    -DCMAKE_MODULE_PATH="${ROOT_DIR}/native/macos/cmake" \
    -DCMAKE_C_COMPILER="${CC_BIN}" \
    -DCMAKE_CXX_COMPILER="${CXX_BIN}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_OSX_ARCHITECTURES="${target_arch}" \
    -DCMAKE_OSX_DEPLOYMENT_TARGET="${DEPLOYMENT_TARGET}" \
    -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
    -DBUILD_SHARED_LIBS=OFF \
    -DJPEGXL_STATIC=ON \
    -DJPEGXL_ENABLE_SKCMS=ON \
    -DJPEGXL_FORCE_SYSTEM_LCMS2=OFF \
    -DJPEGXL_FORCE_SYSTEM_HWY=OFF \
    -DJPEGXL_FORCE_SYSTEM_BROTLI=OFF \
    -DJPEGXL_ENABLE_TOOLS=OFF \
    -DJPEGXL_ENABLE_EXAMPLES=OFF \
    -DBUILD_TESTING=OFF \
    -DJPEGXL_ENABLE_TESTS=OFF \
    -DJPEGXL_ENABLE_BENCHMARK=OFF \
    -DJPEGXL_ENABLE_DEVTOOLS=OFF \
    -DJPEGXL_ENABLE_DOXYGEN=OFF \
    -DJPEGXL_ENABLE_MANPAGES=OFF \
    -DJPEGXL_ENABLE_PLUGINS=OFF \
    -DJPEGXL_ENABLE_OPENEXR=OFF \
    -DJPEGXL_ENABLE_SJPEG=OFF \
    -DJPEGXL_ENABLE_JPEGLI=OFF \
    -DJPEGXL_ENABLE_JPEGLI_LIBJPEG=OFF \
    -DCMAKE_DISABLE_FIND_PACKAGE_PkgConfig=TRUE \
    -DCMAKE_DISABLE_FIND_PACKAGE_PNG=TRUE \
    -DCMAKE_DISABLE_FIND_PACKAGE_GIF=TRUE \
    -DCMAKE_DISABLE_FIND_PACKAGE_JPEG=TRUE \
    -DCMAKE_FIND_USE_PACKAGE_REGISTRY=OFF \
    -DCMAKE_FIND_USE_SYSTEM_PACKAGE_REGISTRY=OFF \
    -DCMAKE_IGNORE_PATH="/opt/homebrew;/usr/local"
  "${CMAKE_BIN}" --build "${jxl_build}" --config Release -j"${JOBS}"

  "${CMAKE_BIN}" -S "${ROOT_DIR}/native/macos" -B "${native_build}" -G Ninja \
    -DCMAKE_MAKE_PROGRAM="${NINJA_BIN}" \
    -DCMAKE_C_COMPILER="${CC_BIN}" \
    -DCMAKE_CXX_COMPILER="${CXX_BIN}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_OSX_ARCHITECTURES="${target_arch}" \
    -DCMAKE_OSX_DEPLOYMENT_TARGET="${DEPLOYMENT_TARGET}" \
    -DCMAKE_MODULE_PATH="${ROOT_DIR}/native/macos/cmake" \
    -DPS_LIBRAW_ROOT="${LIBRAW_ROOT}" \
    -DPS_DNG_ROOT="${DNG_ROOT}" \
    -DPS_XMP_ROOT="${XMP_ROOT}" \
    -DPS_JXL_SOURCE_ROOT="${JXL_SOURCE_ROOT}" \
    -DPS_JXL_BUILD_ROOT="${jxl_build}" \
    -DPS_JPEG_INCLUDE_DIR="${jpeg_prefix}/include" \
    -DPS_JPEG_LIBRARY="${jpeg_prefix}/lib/libjpeg.a" \
    -DPS_OUTPUT_DIR="${native_stage}" \
    -DCMAKE_IGNORE_PATH="/opt/homebrew;/usr/local"
  "${CMAKE_BIN}" --build "${native_build}" --config Release -j"${JOBS}"

  local artifact="${native_stage}/libphotosheet_raw.dylib"
  [[ -f "${artifact}" ]] || { echo "native dylib 未生成：${artifact}" >&2; exit 1; }
  if otool -L "${artifact}" | grep -E '/opt/homebrew|/usr/local' >/dev/null; then
    echo "native dylib 带有不允许的 Homebrew/local 动态依赖：${artifact}" >&2
    otool -L "${artifact}" >&2
    exit 1
  fi
  install -m 0755 "${artifact}" "${destination}"
  echo "已安装：${destination}"
  file "${destination}"
  shasum -a 256 "${destination}"
}

if [[ "${ARCH}" == "both" ]]; then
  build_one arm64
  build_one amd64
else
  build_one "${ARCH}"
fi

echo "macOS 原生 RAW backend 构建完成：${OUTPUT_ROOT}"
