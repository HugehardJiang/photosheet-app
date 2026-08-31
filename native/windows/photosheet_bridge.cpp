// PhotoSheet's narrow C ABI around the embedded LibRaw/DNG SDK build.
// This file is copied into the LibRaw project during the native build so the
// final DLL contains one allocation domain and can be embedded into the EXE.
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <exception>
#include <new>

#include "../../libraw/libraw.h"
#include "dng_host.h"

struct photosheet_raw_info {
  uint32_t width;
  uint32_t height;
  uint32_t channels;
  uint32_t bits;
  uint32_t orientation;
};

static void set_error(char *buffer, uint64_t capacity, const char *message) {
  if (!buffer || capacity == 0) return;
  if (!message) message = "unknown native RAW error";
  std::snprintf(buffer, static_cast<size_t>(capacity), "%s", message);
  buffer[capacity - 1] = '\0';
}

// Capability bits are deliberately part of the tiny ABI so Go can reject a
// stale or accidentally misconfigured DLL at startup instead of discovering
// a missing DNG SDK/JPEG XL path only after the first user import.
enum photosheet_raw_capability_flags : uint32_t {
  PHOTOSHEET_CAP_DNGSDK = 1u << 0,
  PHOTOSHEET_CAP_JPEGXL = 1u << 1,
  PHOTOSHEET_CAP_16BIT = 1u << 2,
  PHOTOSHEET_CAP_DNG_OPCODE = 1u << 3,
  PHOTOSHEET_CAP_SRGB = 1u << 4,
  PHOTOSHEET_CAP_UNICODE_PATHS = 1u << 5,
};

extern "C" __declspec(dllexport) int photosheet_raw_capabilities(
    uint32_t *flags, char *error, uint64_t error_capacity) {
  if (!flags) {
    set_error(error, error_capacity, "invalid native capability arguments");
    return -1000;
  }
  *flags = PHOTOSHEET_CAP_DNGSDK | PHOTOSHEET_CAP_JPEGXL |
           PHOTOSHEET_CAP_16BIT | PHOTOSHEET_CAP_DNG_OPCODE |
           PHOTOSHEET_CAP_SRGB | PHOTOSHEET_CAP_UNICODE_PATHS;
  return LIBRAW_SUCCESS;
}

extern "C" __declspec(dllexport) int photosheet_raw_decode_w(
    const wchar_t *path, unsigned char **data, uint64_t *data_size,
    photosheet_raw_info *info, char *error, uint64_t error_capacity) {
  if (!path || !data || !data_size || !info) {
    set_error(error, error_capacity, "invalid native RAW decode arguments");
    return -1000;
  }
  *data = nullptr;
  *data_size = 0;
  std::memset(info, 0, sizeof(*info));
  try {
    LibRaw raw;
    dng_host host;
    raw.set_dng_host(&host);
    raw.imgdata.rawparams.use_dngsdk = LIBRAW_DNG_ALL;
    raw.imgdata.rawparams.options |= LIBRAW_RAWOPTIONS_DNG_STAGE23_IFPRESENT_JPGJXL;
    raw.imgdata.params.output_bps = 16;
    raw.imgdata.params.use_camera_wb = 1;
    raw.imgdata.params.no_auto_bright = 1;
    raw.imgdata.params.output_color = 1;
    raw.imgdata.params.output_tiff = 0;
    raw.imgdata.params.half_size = 0;

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
    const uint64_t bytes = image->data_size;
    unsigned char *copy = static_cast<unsigned char *>(std::malloc(static_cast<size_t>(bytes)));
    if (!copy) {
      LibRaw::dcraw_clear_mem(image);
      set_error(error, error_capacity, "native RAW output allocation failed");
      return -1002;
    }
    std::memcpy(copy, image->data, static_cast<size_t>(bytes));
    info->width = image->width;
    info->height = image->height;
    info->channels = image->colors;
    info->bits = image->bits;
    info->orientation = raw.imgdata.sizes.flip;
    *data = copy;
    *data_size = bytes;
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

extern "C" __declspec(dllexport) int photosheet_raw_probe_w(
    const wchar_t *path, photosheet_raw_info *info, char *error,
    uint64_t error_capacity) {
  if (!path || !info) {
    set_error(error, error_capacity, "invalid native RAW probe arguments");
    return -1000;
  }
  std::memset(info, 0, sizeof(*info));
  try {
    LibRaw raw;
    dng_host host;
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
  } catch (const std::exception &exception) {
    set_error(error, error_capacity, exception.what());
    return -1003;
  } catch (...) {
    set_error(error, error_capacity, "native RAW probe exception");
    return -1003;
  }
}

extern "C" __declspec(dllexport) void photosheet_raw_free(void *data) {
  std::free(data);
}
