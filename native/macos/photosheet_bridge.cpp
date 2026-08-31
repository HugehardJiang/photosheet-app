// PhotoSheet's UTF-8 C ABI around the macOS LibRaw/DNG SDK build.
// All native pixel allocations are made with malloc and released through the
// same dylib, so Go never crosses an allocation domain.
#include "photosheet_bridge.h"

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <exception>
#include <limits>
#include <new>

#include "dng_host.h"
#include "libraw.h"

static_assert(sizeof(photosheet_raw_info) == 20, "PhotoSheet native ABI mismatch");

#if defined(__GNUC__) || defined(__clang__)
#define PHOTOSHEET_EXPORT __attribute__((visibility("default")))
#else
#define PHOTOSHEET_EXPORT
#endif

static void set_error(char *buffer, uint64_t capacity, const char *message) {
  if (!buffer || capacity == 0) return;
  if (!message) message = "unknown native RAW error";
  std::snprintf(buffer, static_cast<size_t>(capacity), "%s", message);
  buffer[capacity - 1] = '\0';
}

enum photosheet_raw_capability_flags : uint32_t {
  PHOTOSHEET_CAP_DNGSDK = 1u << 0,
  PHOTOSHEET_CAP_JPEGXL = 1u << 1,
  PHOTOSHEET_CAP_16BIT = 1u << 2,
  PHOTOSHEET_CAP_DNG_OPCODE = 1u << 3,
  PHOTOSHEET_CAP_SRGB = 1u << 4,
  PHOTOSHEET_CAP_UNICODE_PATHS = 1u << 5,
};

static void configure_raw(LibRaw &raw, dng_host &host) {
  raw.set_dng_host(&host);
  raw.imgdata.rawparams.use_dngsdk = LIBRAW_DNG_ALL;
  raw.imgdata.rawparams.options |= LIBRAW_RAWOPTIONS_DNG_STAGE23_IFPRESENT_JPGJXL;
  raw.imgdata.params.output_bps = 16;
  raw.imgdata.params.use_camera_wb = 1;
  raw.imgdata.params.no_auto_bright = 1;
  raw.imgdata.params.output_color = 1;  // sRGB
  raw.imgdata.params.output_tiff = 0;
  raw.imgdata.params.half_size = 0;
}

extern "C" PHOTOSHEET_EXPORT int photosheet_raw_capabilities(
    uint32_t *flags, char *error, uint64_t error_capacity) {
  if (!flags) {
    set_error(error, error_capacity, "invalid native capability arguments");
    return -1000;
  }
  *flags = 0;
  try {
    LibRaw raw;
    const unsigned libraw_flags = raw.capabilities();
#ifdef USE_DNGSDK
    if ((libraw_flags & LIBRAW_CAPS_DNGSDK) != 0) {
      *flags |= PHOTOSHEET_CAP_DNGSDK;
    }
#endif
#ifdef qDNGSupportJXL
    *flags |= PHOTOSHEET_CAP_JPEGXL;
#endif
#ifdef USE_DNGSDK
    *flags |= PHOTOSHEET_CAP_DNG_OPCODE;
#endif
    // LibRaw's processed-image API supports 16-bit output and sRGB output
    // independently of the input file's bit depth.
    *flags |= PHOTOSHEET_CAP_16BIT | PHOTOSHEET_CAP_SRGB |
              PHOTOSHEET_CAP_UNICODE_PATHS;
    return LIBRAW_SUCCESS;
  } catch (const std::exception &exception) {
    set_error(error, error_capacity, exception.what());
    return -1003;
  } catch (...) {
    set_error(error, error_capacity, "native capability self-test exception");
    return -1003;
  }
}

extern "C" PHOTOSHEET_EXPORT int photosheet_raw_decode_utf8(
    const char *path, unsigned char **data, uint64_t *data_size,
    photosheet_raw_info *info, char *error, uint64_t error_capacity) {
  if (!path || !data || !data_size || !info) {
    set_error(error, error_capacity, "invalid native RAW decode arguments");
    return -1000;
  }
  *data = nullptr;
  *data_size = 0;
  std::memset(info, 0, sizeof(*info));
  try {
    dng_host host;
    LibRaw raw;
    configure_raw(raw, host);

    int rc = raw.open_file(path);
    if (rc != LIBRAW_SUCCESS) {
      set_error(error, error_capacity, raw.strerror(rc));
      return rc;
    }
    rc = raw.unpack();
    if (rc != LIBRAW_SUCCESS) {
      set_error(error, error_capacity, raw.strerror(rc));
      return rc;
    }
    rc = raw.dcraw_process();
    if (rc != LIBRAW_SUCCESS) {
      set_error(error, error_capacity, raw.strerror(rc));
      return rc;
    }

    int image_error = LIBRAW_SUCCESS;
    libraw_processed_image_t *image = raw.dcraw_make_mem_image(&image_error);
    if (!image) {
      set_error(error, error_capacity, raw.strerror(image_error));
      return image_error == LIBRAW_SUCCESS ? -1001 : image_error;
    }
    const uint64_t expected =
        static_cast<uint64_t>(image->width) *
        static_cast<uint64_t>(image->height) * 3u * sizeof(uint16_t);
    if (image->colors != 3 || image->bits != 16 || image->data_size != expected ||
        expected == 0) {
      LibRaw::dcraw_clear_mem(image);
      set_error(error, error_capacity, "native RAW output is not RGB16");
      return -1004;
    }
    if (expected > static_cast<uint64_t>(std::numeric_limits<size_t>::max())) {
      LibRaw::dcraw_clear_mem(image);
      set_error(error, error_capacity, "native RAW output is too large");
      return -1002;
    }

    unsigned char *copy =
        static_cast<unsigned char *>(std::malloc(static_cast<size_t>(expected)));
    if (!copy) {
      LibRaw::dcraw_clear_mem(image);
      set_error(error, error_capacity, "native RAW output allocation failed");
      return -1002;
    }
    std::memcpy(copy, image->data, static_cast<size_t>(expected));
    info->width = image->width;
    info->height = image->height;
    info->channels = image->colors;
    info->bits = image->bits;
    info->orientation = raw.imgdata.sizes.flip;
    *data = copy;
    *data_size = expected;
    LibRaw::dcraw_clear_mem(image);
    return LIBRAW_SUCCESS;
  } catch (const std::bad_alloc &) {
    set_error(error, error_capacity, "native RAW output allocation failed");
    return -1002;
  } catch (const std::exception &exception) {
    set_error(error, error_capacity, exception.what());
    return -1003;
  } catch (...) {
    set_error(error, error_capacity, "native RAW exception");
    return -1003;
  }
}

extern "C" PHOTOSHEET_EXPORT int photosheet_raw_probe_utf8(
    const char *path, photosheet_raw_info *info, char *error,
    uint64_t error_capacity) {
  if (!path || !info) {
    set_error(error, error_capacity, "invalid native RAW probe arguments");
    return -1000;
  }
  std::memset(info, 0, sizeof(*info));
  try {
    dng_host host;
    LibRaw raw;
    raw.set_dng_host(&host);
    int rc = raw.open_file(path);
    if (rc != LIBRAW_SUCCESS) {
      set_error(error, error_capacity, raw.strerror(rc));
      return rc;
    }
    info->width = raw.imgdata.sizes.width;
    info->height = raw.imgdata.sizes.height;
    info->channels = raw.imgdata.idata.colors;
    info->bits = raw.imgdata.rawdata.color.maximum > 0 ? 16 : 0;
    info->orientation = raw.imgdata.sizes.flip;
    return LIBRAW_SUCCESS;
  } catch (const std::bad_alloc &) {
    set_error(error, error_capacity, "native RAW probe allocation failed");
    return -1002;
  } catch (const std::exception &exception) {
    set_error(error, error_capacity, exception.what());
    return -1003;
  } catch (...) {
    set_error(error, error_capacity, "native RAW probe exception");
    return -1003;
  }
}

extern "C" PHOTOSHEET_EXPORT void photosheet_raw_free(void *data) {
  std::free(data);
}
