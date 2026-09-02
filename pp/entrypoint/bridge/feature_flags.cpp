#include "feature_flags.h"

#include "entrypoint/types/feature_flags.h"
#include "entrypoint/types/feature_flags_config.h"

extern "C" void prompp_feature_flags_initialize(void* args) {
  struct Arguments {
    PromppFeatures features;
  };

  const auto in = static_cast<Arguments*>(args);
  entrypoint::types::feature_flags().initialize(in->features);
}
