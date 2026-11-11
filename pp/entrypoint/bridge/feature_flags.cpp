#include "feature_flags.h"
#include "annotations.hpp"

#include "entrypoint/types/feature_flags.h"
#include "entrypoint/types/feature_flags_config.h"

/**
 * @brief Initialize C++ feature flags
 *
 * @param args {
 *     features PromppFeatures
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_feature_flags_initialize(void* args) {
  struct Arguments {
    PromppFeatures features;
  };

  const auto in = static_cast<Arguments*>(args);
  entrypoint::types::feature_flags().initialize(in->features);
}
