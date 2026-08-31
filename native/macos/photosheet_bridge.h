#ifndef PHOTOSHEET_RAW_MACOS_BRIDGE_H
#define PHOTOSHEET_RAW_MACOS_BRIDGE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct photosheet_raw_info {
  uint32_t width;
  uint32_t height;
  uint32_t channels;
  uint32_t bits;
  uint32_t orientation;
} photosheet_raw_info;

int photosheet_raw_capabilities(uint32_t *flags, char *error,
                                uint64_t error_capacity);

int photosheet_raw_probe_utf8(const char *path, photosheet_raw_info *info,
                              char *error, uint64_t error_capacity);

int photosheet_raw_decode_utf8(const char *path, unsigned char **data,
                               uint64_t *data_size, photosheet_raw_info *info,
                               char *error, uint64_t error_capacity);

void photosheet_raw_free(void *data);

#ifdef __cplusplus
}
#endif

#endif
