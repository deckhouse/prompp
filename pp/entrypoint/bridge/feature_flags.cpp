#include "feature_flags.h"

#include "entrypoint/types/feature_flags.h"

extern "C" void prompp_feature_flags_initialize(void* args) {
  struct Arguments {
    uint64_t enabled_features;
  };

  const auto in = static_cast<Arguments*>(args);
  entrypoint::types::feature_flags().initialize(in->enabled_features);
}
