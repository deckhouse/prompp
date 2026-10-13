#pragma once

#ifndef __cplusplus
#include <stdbool.h>
#endif

typedef struct {
  bool scraper_validate_utf_per_token;
  bool skip_no_samples_series;
} PromppFeatures;
