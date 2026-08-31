cmake_minimum_required(VERSION 3.22)

project(PhotoSheetRawNative LANGUAGES C CXX)

if(NOT APPLE)
  message(FATAL_ERROR "The PhotoSheet macOS native backend must be built on Apple platforms")
endif()

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

set(PS_LIBRAW_ROOT "" CACHE PATH "LibRaw 0.22.2 source directory")
set(PS_DNG_ROOT "" CACHE PATH "Adobe DNG SDK source directory")
set(PS_XMP_ROOT "" CACHE PATH "Adobe XMP Toolkit source directory")
set(PS_JXL_SOURCE_ROOT "" CACHE PATH "Bundled libjxl source directory")
set(PS_JXL_BUILD_ROOT "" CACHE PATH "libjxl CMake build directory")
set(PS_JPEG_INCLUDE_DIR "" CACHE PATH "libjpeg include directory")
set(PS_JPEG_LIBRARY "" CACHE FILEPATH "libjpeg static archive")
set(PS_OUTPUT_DIR "${CMAKE_BINARY_DIR}/output" CACHE PATH "Native artifact output directory")

foreach(required_path
    PS_LIBRAW_ROOT
    PS_DNG_ROOT
    PS_XMP_ROOT
    PS_JXL_SOURCE_ROOT
    PS_JXL_BUILD_ROOT
    PS_JPEG_INCLUDE_DIR
    PS_JPEG_LIBRARY)
  if(NOT EXISTS "${${required_path}}")
    message(FATAL_ERROR "${required_path} is required and must exist")
  endif()
endforeach()

set(PS_XMP_BUILD_ROOT "${PS_XMP_ROOT}/build")
set(PS_XMP_SOURCE_ROOT "${PS_XMP_ROOT}/XMPCore/source")
set(PS_XMP_EXPAT_ROOT "${PS_XMP_ROOT}/XMPCore/third-party/expat/public/lib")
set(PS_XMP_RESOURCE_ROOT "${PS_XMP_ROOT}/XMPCore/resource/mac")
set(PS_XMP_ZUID_ROOT "${PS_XMP_ROOT}/third-party/zuid")

foreach(required_path
    PS_XMP_BUILD_ROOT
    PS_XMP_SOURCE_ROOT
    PS_XMP_EXPAT_ROOT
    PS_XMP_RESOURCE_ROOT
    PS_XMP_ZUID_ROOT)
  if(NOT EXISTS "${${required_path}}")
    message(FATAL_ERROR "XMP source layout is incomplete: ${${required_path}}")
  endif()
endforeach()

file(GLOB PS_XMP_ABI_SOURCES CONFIGURE_DEPENDS
  "${PS_XMP_SOURCE_ROOT}/WXMP*.cpp")
file(GLOB PS_XMP_CORE_SOURCES CONFIGURE_DEPENDS
  "${PS_XMP_SOURCE_ROOT}/XMP*.cpp")
list(REMOVE_ITEM PS_XMP_CORE_SOURCES
  "${PS_XMP_SOURCE_ROOT}/XMPCore_Impl.cpp")
file(GLOB PS_XMP_ZUID_SOURCES CONFIGURE_DEPENDS
  "${PS_XMP_ZUID_ROOT}/sources/*.cpp")

set(PS_XMP_SOURCES
  ${PS_XMP_ABI_SOURCES}
  ${PS_XMP_CORE_SOURCES}
  ${PS_XMP_ZUID_SOURCES}
  "${PS_XMP_SOURCE_ROOT}/CoreObjectFactoryImpl.cpp"
  "${PS_XMP_EXPAT_ROOT}/xmlparse.c"
  "${PS_XMP_EXPAT_ROOT}/xmlrole.c"
  "${PS_XMP_EXPAT_ROOT}/xmltok.c"
  "${PS_XMP_SOURCE_ROOT}/ExpatAdapter.cpp"
  "${PS_XMP_SOURCE_ROOT}/ParseRDF.cpp"
  "${PS_XMP_SOURCE_ROOT}/XMPCore_Impl.cpp"
  "${PS_XMP_ROOT}/source/UnicodeConversions.cpp"
  "${PS_XMP_ROOT}/source/XML_Node.cpp"
  "${PS_XMP_ROOT}/source/XMP_LibUtils.cpp")

add_library(photosheet_xmpcore STATIC ${PS_XMP_SOURCES})
target_include_directories(photosheet_xmpcore PRIVATE
  "${PS_XMP_ROOT}"
  "${PS_XMP_ROOT}/public/include"
  "${PS_XMP_ROOT}/third-party/expat/public/lib"
  "${PS_XMP_RESOURCE_ROOT}"
  "${PS_XMP_BUILD_ROOT}"
  "${PS_XMP_ZUID_ROOT}/interfaces")
target_compile_definitions(photosheet_xmpcore PRIVATE
  AdobePrivate=1
  BUILDING_XMPCORE_LIB=1
  BUILDING_XMPCORE_AS_STATIC=1
  ENABLE_CPP_DOM_MODEL=0
  HAVE_EXPAT_CONFIG_H=1
  MAC_ENV=1
  Macintosh=1
  XML_STATIC=1
  XMP_64=1
  XMP_StaticBuild=1
  XMP_COMPONENT_INT_NAMESPACE=AdobeXMPCore_Int)
set_source_files_properties(
  "${PS_XMP_EXPAT_ROOT}/xmlparse.c"
  "${PS_XMP_EXPAT_ROOT}/xmlrole.c"
  "${PS_XMP_EXPAT_ROOT}/xmltok.c"
  PROPERTIES LANGUAGE C)

file(GLOB PS_DNG_SOURCES CONFIGURE_DEPENDS
  "${PS_DNG_ROOT}/*.cpp")
list(REMOVE_ITEM PS_DNG_SOURCES "${PS_DNG_ROOT}/dng_validate.cpp")

add_library(photosheet_dngsdk STATIC ${PS_DNG_SOURCES})
target_include_directories(photosheet_dngsdk PRIVATE
  "${PS_DNG_ROOT}"
  "${PS_JPEG_INCLUDE_DIR}"
  "${PS_JXL_BUILD_ROOT}/lib/include"
  "${PS_JXL_SOURCE_ROOT}/third_party/highway"
  "${PS_JXL_SOURCE_ROOT}/third_party/brotli/c/include"
  "${PS_XMP_ROOT}/public/include"
  "${PS_XMP_ROOT}/build")
target_compile_definitions(photosheet_dngsdk PRIVATE
  ENABLE_CPP_DOM_MODEL=0
  HAVE_EXPAT_CONFIG_H=1
  MAC_ENV=1
  Macintosh=1
  USE_DNGSDK=1
  XML_STATIC=1
  XMP_StaticBuild=1
  qDNG64Bit=1
  qDNGDebug=0
  qDNGSupportJXL=1
  qDNGUseLibJPEG=1
  qDNGUseStdInt=1
  qDNGUseXMP=1
  qDNGValidate=0
  qDNGValidateTarget=0
  qDNGXMPDocOps=0
  qDNGXMPFiles=0
  qMacOS=1
  qWinOS=0)
target_link_libraries(photosheet_dngsdk PRIVATE
  photosheet_xmpcore
  "${PS_JPEG_LIBRARY}")

file(GLOB_RECURSE PS_LIBRAW_SOURCES CONFIGURE_DEPENDS
  "${PS_LIBRAW_ROOT}/src/*.cpp")
# LibRaw ships a few historical single-file implementations alongside the
# normal split sources. The upstream Makefile selects one variant; compiling
# every .cpp would create duplicate symbols on Darwin (and on any strict
# two-level linker).
list(REMOVE_ITEM PS_LIBRAW_SOURCES
  "${PS_LIBRAW_ROOT}/src/preprocessing/preprocessing_ph.cpp"
  "${PS_LIBRAW_ROOT}/src/postprocessing/postprocessing_ph.cpp"
  "${PS_LIBRAW_ROOT}/src/write/write_ph.cpp")

add_library(photosheet_raw SHARED
  ${PS_LIBRAW_SOURCES}
  "${CMAKE_CURRENT_SOURCE_DIR}/photosheet_bridge.cpp")
target_include_directories(photosheet_raw PRIVATE
  "${PS_LIBRAW_ROOT}"
  "${PS_LIBRAW_ROOT}/libraw"
  "${PS_LIBRAW_ROOT}/internal"
  "${PS_DNG_ROOT}"
  "${PS_JPEG_INCLUDE_DIR}"
  "${PS_JXL_BUILD_ROOT}/lib/include"
  "${PS_JXL_SOURCE_ROOT}/third_party/highway"
  "${PS_JXL_SOURCE_ROOT}/third_party/brotli/c/include"
  "${CMAKE_CURRENT_SOURCE_DIR}")
target_compile_definitions(photosheet_raw PRIVATE
  ENABLE_CPP_DOM_MODEL=0
  HAVE_EXPAT_CONFIG_H=1
  MAC_ENV=1
  Macintosh=1
  USE_DNGSDK=1
  USE_JPEG=1
  USE_ZLIB=1
  XML_STATIC=1
  XMP_StaticBuild=1
  qDNG64Bit=1
  qDNGSupportJXL=1
  qDNGUseLibJPEG=1
  qDNGUseStdInt=1
  qDNGUseXMP=1
  qDNGValidate=0
  qDNGValidateTarget=0
  qDNGXMPDocOps=0
  qDNGXMPFiles=0
  qMacOS=1
  qWinOS=0)
target_compile_options(photosheet_raw PRIVATE
  -Wno-macro-redefined
  -fvisibility=hidden
  -fvisibility-inlines-hidden)
target_link_libraries(photosheet_raw PRIVATE
  photosheet_dngsdk
  photosheet_xmpcore
  "${PS_JPEG_LIBRARY}"
  "${PS_JXL_BUILD_ROOT}/lib/libjxl.a"
  "${PS_JXL_BUILD_ROOT}/lib/libjxl_threads.a"
  "${PS_JXL_BUILD_ROOT}/lib/libjxl_cms.a"
  "${PS_JXL_BUILD_ROOT}/third_party/highway/libhwy.a"
  "${PS_JXL_BUILD_ROOT}/third_party/brotli/libbrotlidec.a"
  "${PS_JXL_BUILD_ROOT}/third_party/brotli/libbrotlienc.a"
  "${PS_JXL_BUILD_ROOT}/third_party/brotli/libbrotlicommon.a"
  z)
set_target_properties(photosheet_raw PROPERTIES
  OUTPUT_NAME "photosheet_raw"
  PREFIX "lib"
  SUFFIX ".dylib"
  MACOSX_RPATH ON
  INSTALL_NAME_DIR "@rpath"
  LIBRARY_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/lib")

add_custom_command(TARGET photosheet_raw POST_BUILD
  COMMAND "${CMAKE_COMMAND}" -E make_directory "${PS_OUTPUT_DIR}"
  COMMAND "${CMAKE_COMMAND}" -E copy "$<TARGET_FILE:photosheet_raw>"
  "${PS_OUTPUT_DIR}/libphotosheet_raw.dylib"
  COMMENT "Staging PhotoSheet macOS native RAW backend")

target_link_options(photosheet_raw PRIVATE
  "-Wl,-framework,CoreFoundation"
  "-Wl,-framework,CoreServices"
  "-Wl,-framework,Carbon")

message(STATUS "PhotoSheet macOS native backend configured")
message(STATUS "  LibRaw: ${PS_LIBRAW_ROOT}")
message(STATUS "  DNG SDK: ${PS_DNG_ROOT}")
message(STATUS "  XMP Core: ${PS_XMP_ROOT}")
message(STATUS "  libjxl: ${PS_JXL_BUILD_ROOT}")
message(STATUS "  output: ${PS_OUTPUT_DIR}")
