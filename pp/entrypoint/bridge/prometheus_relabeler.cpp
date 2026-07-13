#include "prometheus_relabeler.h"
#include "annotations.h"

#include "entrypoint/types/exception.h"
#include "entrypoint/types/hashdex.h"
#include "entrypoint/types/lss.h"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

using entrypoint::types::LssVariantPtr;
using PromPP::Primitives::Go::SliceView;
using PromPP::Prometheus::Relabel::InnerSeries;
using PromPP::Prometheus::Relabel::RelabeledSeries;
using PromPP::Prometheus::Relabel::RelabelerStateUpdate;

//
// StatelessRelabeler
//

using Cache = PromPP::Prometheus::Relabel::Cache;
using CachePtr = std::unique_ptr<Cache>;

using StatelessRelabeler = PromPP::Prometheus::Relabel::StatelessRelabeler;
using StatelessRelabelerPtr = std::unique_ptr<StatelessRelabeler>;

/**
 * @brief Construct a new StatelessRelabeler.
 *
 * @param args {
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     error               []byte    // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
  };
  struct Result {
    StatelessRelabelerPtr stateless_relabeler;
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    out->stateless_relabeler = std::make_unique<StatelessRelabeler>(in->go_rcfgs);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Destroy StatelessRelabeler
 *
 * @param args {
 *     stateless_relabeler uintptr // pointer of StatelessRelabeler;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_stateless_relabeler_dtor(void* args) {
  struct Arguments {
    StatelessRelabelerPtr stateless_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief reset_to reset configs and replace on new converting go-config.
 *
 * @param args {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     error               []byte    // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_stateless_relabeler_reset_to(void* args, void* res) {
  struct Arguments {
    StatelessRelabelerPtr stateless_relabeler;
    SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);

  try {
    in->stateless_relabeler->reset_to(in->go_rcfgs);
  } catch (...) {
    auto* out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

//
// InnerSeries
//

/**
 * @brief initialize slice of InnerSeries
 *
 * @param args {
 *     innerSeries []InnerSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_inner_series_ctor(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
  };

  auto& inner_series = static_cast<Arguments*>(args)->inner_series;
  std::uninitialized_default_construct_n(inner_series.begin(), inner_series.size());
}

/**
 * @brief Destroy slice of InnerSeries
 *
 * @param args {
 *      innerSeries []InnerSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_inner_series_dtor(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
  };

  auto& inner_series = static_cast<Arguments*>(args)->inner_series;
  std::destroy_n(inner_series.begin(), inner_series.size());
}

/**
 * @brief Reset slice of InnerSeries
 *
 * @param args {
 *      innerSeries []InnerSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_inner_series_reset(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
  };

  auto& inner_series = static_cast<Arguments*>(args)->inner_series;
  for (auto& series : inner_series) {
    series.reset();
  }
}

//
// RelabeledSeries
//

/**
 * @brief initialize slice of RelabeledSeries
 *
 * @param args {
 *     relabeledSeries []RelabeledSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeled_series_ctor(void* args) {
  struct Arguments {
    SliceView<RelabeledSeries> relabeled_series;
  };

  auto& relabeled_series = static_cast<Arguments*>(args)->relabeled_series;
  std::uninitialized_default_construct_n(relabeled_series.begin(), relabeled_series.size());
}

/**
 * @brief Destroy slice of RelabeledSeries
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeled_series_dtor(void* args) {
  struct Arguments {
    SliceView<RelabeledSeries> relabeled_series;
  };

  auto& relabeled_series = static_cast<Arguments*>(args)->relabeled_series;
  std::destroy_n(relabeled_series.begin(), relabeled_series.size());
}

/**
 * @brief Reset slice of RelabeledSeries
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeled_series_reset(void* args) {
  struct Arguments {
    SliceView<RelabeledSeries> relabeled_series;
  };

  auto& relabeled_series = static_cast<Arguments*>(args)->relabeled_series;
  for (auto& series : relabeled_series) {
    series.reset();
  }
}

//
// RelabelerStateUpdate
//

/**
 * @brief Initialize slice of RelabelerStateUpdate.
 *
 * @param args {
 *     relabeler_state_update []RelabelerStateUpdate
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeler_state_update_ctor(void* args) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> relabeler_state_update;
  };

  auto& relabeler_state_update = static_cast<Arguments*>(args)->relabeler_state_update;
  std::uninitialized_default_construct_n(relabeler_state_update.begin(), relabeler_state_update.size());
}

/**
 * @brief Destroy slice of RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeler_state_update_dtor(void* args) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> relabeler_state_update;
  };

  auto& relabeler_state_update = static_cast<Arguments*>(args)->relabeler_state_update;
  std::destroy_n(relabeler_state_update.begin(), relabeler_state_update.size());
}

/**
 * @brief Reset slice of RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabeler_state_update_reset(void* args) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> relabeler_state_update;
  };

  auto& relabeler_state_update = static_cast<Arguments*>(args)->relabeler_state_update;
  for (auto& state_update : relabeler_state_update) {
    state_update.clear();
  }
}

//
// PerShardRelabeler
//

using PerShardRelabeler = PromPP::Prometheus::Relabel::PerShardRelabeler;
using PerShardRelabelerPtr = std::unique_ptr<PerShardRelabeler>;

/**
 * @brief Construct a new PerShardRelabeler.
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     shard_id            uint16  // current shard id;
 *     log_shards          uint8   // logarithm to the base 2 of total shards count;
 * }
 *
 * @param res {
 *     per_shard_relabeler uintptr // pointer to constructed PerShardRelabeler;
 *     error               []byte  // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    uint16_t number_of_shards;
    uint16_t shard_id;
  };
  struct Result {
    PerShardRelabelerPtr per_shard_relabeler;
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto* in = static_cast<Arguments*>(args);
  auto* out = new (res) Result();

  try {
    out->per_shard_relabeler = std::make_unique<PerShardRelabeler>(in->external_labels, in->stateless_relabeler, in->number_of_shards, in->shard_id);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Destroy PerShardRelabeler.
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer of PerShardRelabeler;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_shard_relabeler_dtor(void* args) {
  struct Arguments {
    PerShardRelabelerPtr per_shard_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

using StaleNaNsState = PromPP::Prometheus::Relabel::StaleNaNsState;
using StaleNaNsStatePtr = std::unique_ptr<StaleNaNsState>;

/**
 * @brief Create StaleNaNsState.
 *
 * @param res {
 *     state uintptr // pointer to constructed StaleNaNsState;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabel_stale_nans_state_ctor(void* res) {
  struct Result {
    StaleNaNsStatePtr state;
  };

  new (res) Result{.state = std::make_unique<StaleNaNsState>()};
}

/**
 * @brief Destroy StaleNaNsState.
 *
 * @param args {
 *      state uintptr // pointer to StaleNaNsState;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_relabel_stale_nans_state_dtor(void* args) {
  struct Arguments {
    StaleNaNsStatePtr state;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     cache                  uintptr               // pointer to constructed Cache;
 *     relabeled_shard_id     uint16                // relabeled shard id;
 * }
 *
 * @param res {
 *     error                  []byte  // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_shard_single_relabeler_update_relabeler_state(void* args, void* res) {
  struct Arguments {
    RelabelerStateUpdate* relabeler_state_update;
    PerShardRelabelerPtr per_shard_relabeler;
    CachePtr cache;
    uint16_t relabeled_shard_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);

  try {
    PerShardRelabeler::update_relabeler_state(*in->cache, *in->relabeler_state_update, in->relabeled_shard_id);
  } catch (...) {
    auto* out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief relabeling output series(fourth stage).
 *
 * @param args {
 *     incoming_inner_series     []InnerSeries     // go slice with incoming InnerSeries;
 *     encoders_inner_series     []InnerSeries     // go slice with output InnerSeries;
 *     shards_relabeled_series   []*RelabeledSeries // go slice with output RelabeledSeries;
 *     per_shard_relabeler       uintptr            // pointer to constructed per shard relabeler;
 *     lss                       uintptr            // pointer to constructed label sets;
 *     cache                     uintptr            // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res) {
  struct Arguments {
    RelabeledSeries* relabeled_series;
    SliceView<InnerSeries> incoming_inner_series;
    SliceView<InnerSeries> encoders_inner_series;
    PerShardRelabelerPtr per_shard_relabeler;
    LssVariantPtr lss;
    CachePtr cache;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto in = static_cast<Arguments*>(args);

  try {
    const auto& lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->lss);
    in->per_shard_relabeler->output_relabeling(lss, *in->cache, in->relabeled_series, in->incoming_inner_series, in->encoders_inner_series);
  } catch (...) {
    const auto out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief reset set new number_of_shards and external_labels.
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 *     number_of_shards    uint16  // total shards count;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_shard_relabeler_reset_to(void* args) {
  struct Arguments {
    SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PerShardRelabelerPtr per_shard_relabeler;
    uint16_t number_of_shards;
  };

  const auto* in = static_cast<Arguments*>(args);

  in->per_shard_relabeler->reset_to(in->external_labels, in->number_of_shards);
}

//
// Relabeler cache
//

/**
 * @brief Construct a new Cache.
 *
 * @param res {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_cache_ctor(void* res) {
  struct Result {
    CachePtr cache;
  };

  new (res) Result{.cache = std::make_unique<Cache>()};
}

/**
 * @brief Destroy Cache.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_cache_dtor(void* args) {
  struct Arguments {
    CachePtr cache;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief return size of allocated memory for caches.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     allocated_memory    uint64  // size of allocated memory for label sets;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_cache_allocated_memory(void* args, void* res) {
  struct Arguments {
    CachePtr cache;
  };
  struct Result {
    size_t allocated_memory;
  };

  const auto* in = static_cast<Arguments*>(args);
  new (res) Result{.allocated_memory = in->cache->allocated_memory()};
}

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     shards_relabeler_state_update []*RelabelerStateUpdate // pointer to RelabelerStateUpdate per source shard;
 *     cache                         uintptr                 // pointer to constructed Cache;
 *     relabeled_shard_id            uint16                  // relabeled shard id;
 * }
 *
 * @param res {
 *     error                         []byte                  // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_cache_update(void* args, void* res) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> shards_relabeler_state_update;
    CachePtr cache;
    uint16_t relabeled_shard_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);

  try {
    for (size_t id = 0; id != in->shards_relabeler_state_update.size(); ++id) {
      if (in->shards_relabeler_state_update[id].empty()) {
        continue;
      }

      in->cache->update(in->shards_relabeler_state_update[id], id);
    }
  } catch (...) {
    auto* out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

//
// PerGoroutineRelabeler
//

using PerGoroutineRelabeler = PromPP::Prometheus::Relabel::PerGoroutineRelabeler<SliceView>;
using PerGoroutineRelabelerPtr = std::unique_ptr<PerGoroutineRelabeler>;

/**
 * @brief Construct a new PerGoroutineRelabeler.
 *
 * @param args {
 *     number_of_shards        uint16  // total shards count;
 *     shard_id                uint16  // current shard id;
 * }
 *
 * @param res {
 *     per_goroutine_relabeler uintptr // pointer to constructed PerGoroutineRelabeler;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    uint16_t number_of_shards;
    uint16_t shard_id;
  };
  struct Result {
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
  };

  auto* in = static_cast<Arguments*>(args);
  auto* out = new (res) Result();

  out->per_goroutine_relabeler = std::make_unique<PerGoroutineRelabeler>(in->number_of_shards, in->shard_id);
}

/**
 * @brief Destroy PerGoroutineRelabeler.
 *
 * @param args {
 *     per_goroutine_relabeler uintptr // pointer of PerGoroutineRelabeler;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_dtor(void* args) {
  struct Arguments {
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief relabeling incomig hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series      []RelabeledSeries // go slice with RelabeledSeries;
 *     options                      RelabelerOptions   // object RelabelerOptions;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     stateless_relabeler          uintptr            // pointer to constructed stateless relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     cache                        uintptr            // pointer to constructed Cache;
 *     input_lss                    uintptr            // pointer to constructed input label sets;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_relabeling(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    SliceView<RelabeledSeries> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    HashdexVariant* hashdex;
    CachePtr cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    PromPP::Primitives::Go::Slice<char> error;
    bool target_lss_has_reallocations{};
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<entrypoint::types::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::types::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_relabeling(input_lss, target_lss, *in->cache, hashdex, in->options, *in->stateless_relabeler, *out,
                                                        in->shards_inner_series, in->shards_relabeled_series);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief relabeling incoming hashdex(first stage) from cache.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     options                 RelabelerOptions // object RelabelerOptions;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     cache                   uintptr          // pointer to constructed Cache;
 *     input_lss               uintptr          // pointer to constructed input label sets;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added       uint32               // number of added samples;
 *     series_added        uint32               // number of added series;
 *     series_drop         uint32               // number of dropped series;
 *     ok                  bool                 // true if all label set find in cache;
 *     error               []byte               // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_relabeling_from_cache(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    HashdexVariant* hashdex;
    CachePtr cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    bool ok{};
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<entrypoint::types::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          out->ok =
              in->per_goroutine_relabeler->input_relabeling_from_cache(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief relabeling incoming hashdex(first stage) with state stalenans.
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series      []RelabeledSeries // go slice with RelabeledSeries;
 *     options                      RelabelerOptions   // object RelabelerOptions;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     stateless_relabeler          uintptr            // pointer to constructed stateless relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     cache                        uintptr            // pointer to constructed Cache;
 *     input_lss                    uintptr            // pointer to constructed input label sets;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 *     def_timestamp                int64              // timestamp for metrics and StaleNaNs
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    SliceView<RelabeledSeries> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    HashdexVariant* hashdex;
    CachePtr cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
    PromPP::Primitives::Timestamp def_timestamp;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    PromPP::Primitives::Go::Slice<char> error;
    bool target_lss_has_reallocations{};
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<entrypoint::types::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::types::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_relabeling_with_stalenans(input_lss, target_lss, *in->cache, hashdex, in->options, *in->stateless_relabeler, *out,
                                                                       in->shards_inner_series, in->shards_relabeled_series, in->def_timestamp);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief relabeling incomig hashdex(first stage) from cache with state stalenans.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     options                 RelabelerOptions // object RelabelerOptions;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     cache                   uintptr          // pointer to constructed Cache;
 *     input_lss               uintptr          // pointer to constructed input label sets;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 *     def_timestamp           int64            // timestamp for metrics and StaleNaNs
 * }
 *
 * @param res {
 *     samples_added           uint32           // number of added samples;
 *     series_added            uint32           // number of added series;
 *     series_drop             uint32           // number of dropped series;
 *     ok                      bool             // true if all label set find in cache;
 *     error                   []byte           // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans_from_cache(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    HashdexVariant* hashdex;
    CachePtr cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
    PromPP::Primitives::Timestamp def_timestamp;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    bool ok{};
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<entrypoint::types::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          out->ok = in->per_goroutine_relabeler->input_relabeling_with_stalenans_from_cache(input_lss, target_lss, *in->cache, hashdex, in->options, *out,
                                                                                            in->shards_inner_series, in->def_timestamp);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief transparent relabeling incoming hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    HashdexVariant* hashdex;
    LssVariantPtr target_lss;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    PromPP::Primitives::Go::Slice<char> error;
    bool target_lss_has_reallocations{};
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::types::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_transition_relabeling(target_lss, hashdex, *out, in->shards_inner_series);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief transparent relabeling incomig hashdex(first stage) from cache.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added       uint32               // number of added samples;
 *     series_added        uint32               // number of added series;
 *     series_drop         uint32               // number of dropped series;
 *     ok                  bool                 // true if all label set find in cache;
 *     error               []byte               // error string if thrown;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling_only_read(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    HashdexVariant* hashdex;
    LssVariantPtr target_lss;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    uint32_t series_drop{0};
    bool ok{};
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& target_lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);

          out->ok = in->per_goroutine_relabeler->input_transition_relabeling_only_read(target_lss, hashdex, *out, in->shards_inner_series);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief add relabeled ls to lss, add to result and add to cache update(second stage).
 *
 * @param args {
 *     shards_inner_series           []InnerSeries          // go InnerSeries per source shard;
 *     shards_relabeled_series       []RelabeledSeries      // go RelabeledSeries per source shard;
 *     shards_relabeler_state_update []*RelabelerStateUpdate // pointer to RelabelerStateUpdate per source shard;
 *     per_goroutine_relabeler       uintptr                 // pointer to constructed per goroutine relabeler;
 *     target_lss                    uintptr                 // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                         []byte                  // error string if thrown
 *     target_lss_has_reallocations  bool                    // true if target lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_append_relabeler_series(void* args, void* res) {
  struct Arguments {
    SliceView<InnerSeries> shards_inner_series;
    SliceView<RelabeledSeries> shards_relabeled_series;
    SliceView<RelabelerStateUpdate> shards_relabeler_state_update;
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
    LssVariantPtr target_lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
    bool target_lss_has_reallocations{};
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    auto& lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->target_lss);
    const entrypoint::types::ReallocationsDetector reallocation_detector(lss);

    for (size_t id = 0; id != in->shards_relabeled_series.size(); ++id) {
      if (in->shards_relabeled_series[id].size() == 0) {
        continue;
      }

      PerGoroutineRelabeler::append_relabeler_series(lss, in->shards_inner_series[id], in->shards_relabeled_series[id], in->shards_relabeler_state_update[id]);
    }

    lss.build_deferred_indexes();
    out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief add stale nans to inner series if needed
 *
 * @param args {
 *     inner_series      []InnerSeries // InnerSeries
 *     stale_nan_state   uintptr        // pointer to source state
 *     default_timestamp int64          // timestamp for stale_nan samples
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_prometheus_per_goroutine_relabeler_track_stale_nans(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
    StaleNaNsStatePtr stale_nans_state;
    PromPP::Primitives::Timestamp default_timestamp;
  };

  const auto in = static_cast<Arguments*>(args);
  PerGoroutineRelabeler::track_stale_nans(in->inner_series, *in->stale_nans_state, in->default_timestamp);
}

/**
 * @brief add stale nans to inner series if needed
 *
 * @param args {
 *     stale_nan_state uintptr  // pointer to source state
 *     ls_ids_mapping  uintptr  // pointer to dst_src_ls_ids_mapping
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_remap_stale_nans_state(void* args) {
  struct Arguments {
    StaleNaNsStatePtr stale_nans_state;
    entrypoint::types::LsIdsSlicePtr dst_src_ls_ids_mapping;
  };

  const auto in = static_cast<Arguments*>(args);
  in->stale_nans_state->remap(*in->dst_src_ls_ids_mapping);
}
