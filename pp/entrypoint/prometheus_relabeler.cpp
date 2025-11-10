#include "prometheus_relabeler.h"

#include "exception.hpp"
#include "hashdex.hpp"
#include "head/lss.h"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

using entrypoint::head::LssVariantPtr;
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

extern "C" void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res) {
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
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_stateless_relabeler_dtor(void* args) {
  struct Arguments {
    StatelessRelabelerPtr stateless_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_prometheus_stateless_relabeler_reset_to(void* args, void* res) {
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
    entrypoint::handle_current_exception(err_stream);
  }
}

//
// InnerSeries
//

extern "C" void prompp_prometheus_inner_series_ctor(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
  };

  auto& inner_series = static_cast<Arguments*>(args)->inner_series;
  std::uninitialized_default_construct_n(inner_series.begin(), inner_series.size());
}

extern "C" void prompp_prometheus_inner_series_dtor(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
  };

  auto& inner_series = static_cast<Arguments*>(args)->inner_series;
  std::destroy_n(inner_series.begin(), inner_series.size());
}

//
// RelabeledSeries
//

extern "C" void prompp_prometheus_relabeled_series_ctor(void* args) {
  struct Arguments {
    SliceView<RelabeledSeries> relabeled_series;
  };

  for (auto& series : static_cast<Arguments*>(args)->relabeled_series) {
    std::construct_at(&series);
  }
}

extern "C" void prompp_prometheus_relabeled_series_dtor(void* args) {
  struct Arguments {
    SliceView<RelabeledSeries> relabeled_series;
  };

  for (auto& series : static_cast<Arguments*>(args)->relabeled_series) {
    std::destroy_at(&series);
  }
}

//
// RelabelerStateUpdate
//

extern "C" void prompp_prometheus_relabeler_state_update_ctor(void* args) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> relabeler_state_update;
  };

  for (auto& series : static_cast<Arguments*>(args)->relabeler_state_update) {
    std::construct_at(&series);
  }
}

extern "C" void prompp_prometheus_relabeler_state_update_dtor(void* args) {
  struct Arguments {
    SliceView<RelabelerStateUpdate> relabeler_state_update;
  };

  for (auto& series : static_cast<Arguments*>(args)->relabeler_state_update) {
    std::destroy_at(&series);
  }
}

//
// PerShardRelabeler
//

using PerShardRelabeler = PromPP::Prometheus::Relabel::PerShardRelabeler;
using PerShardRelabelerPtr = std::unique_ptr<PerShardRelabeler>;

extern "C" void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res) {
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
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_dtor(void* args) {
  struct Arguments {
    PerShardRelabelerPtr per_shard_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

using StaleNaNsState = PromPP::Prometheus::Relabel::StaleNaNsState;
using StaleNaNsStatePtr = std::unique_ptr<StaleNaNsState>;

extern "C" void prompp_prometheus_relabel_stale_nans_state_ctor(void* res) {
  struct Result {
    StaleNaNsStatePtr state;
  };

  new (res) Result{.state = std::make_unique<StaleNaNsState>()};
}

extern "C" void prompp_prometheus_relabel_stale_nans_state_dtor(void* args) {
  struct Arguments {
    StaleNaNsStatePtr state;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_prometheus_per_shard_single_relabeler_update_relabeler_state(void* args, void* res) {
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
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res) {
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
    const auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
    in->per_shard_relabeler->output_relabeling(lss, *in->cache, in->relabeled_series, in->incoming_inner_series, in->encoders_inner_series);
  } catch (...) {
    const auto out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_reset_to(void* args) {
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

extern "C" void prompp_prometheus_cache_ctor(void* res) {
  struct Result {
    CachePtr cache;
  };

  new (res) Result{.cache = std::make_unique<Cache>()};
}

extern "C" void prompp_prometheus_cache_dtor(void* args) {
  struct Arguments {
    CachePtr cache;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_prometheus_cache_allocated_memory(void* args, void* res) {
  struct Arguments {
    CachePtr cache;
  };
  struct Result {
    size_t allocated_memory;
  };

  const auto* in = static_cast<Arguments*>(args);
  new (res) Result{.allocated_memory = in->cache->allocated_memory()};
}

extern "C" void prompp_prometheus_cache_update(void* args, void* res) {
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
    entrypoint::handle_current_exception(err_stream);
  }
}

//
// PerGoroutineRelabeler
//

using PerGoroutineRelabeler = PromPP::Prometheus::Relabel::PerGoroutineRelabeler<SliceView>;
using PerGoroutineRelabelerPtr = std::unique_ptr<PerGoroutineRelabeler>;

extern "C" void prompp_prometheus_per_goroutine_relabeler_ctor(void* args, void* res) {
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

extern "C" void prompp_prometheus_per_goroutine_relabeler_dtor(void* args) {
  struct Arguments {
    PerGoroutineRelabelerPtr per_goroutine_relabeler;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_relabeling(void* args, void* res) {
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
          auto& input_lss = std::get<entrypoint::head::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::head::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_relabeling(input_lss, target_lss, *in->cache, hashdex, in->options, *in->stateless_relabeler, *out,
                                                        in->shards_inner_series, in->shards_relabeled_series);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_relabeling_from_cache(void* args, void* res) {
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
          auto& input_lss = std::get<entrypoint::head::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          out->ok =
              in->per_goroutine_relabeler->input_relabeling_from_cache(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans(void* args, void* res) {
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
          auto& input_lss = std::get<entrypoint::head::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::head::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_relabeling_with_stalenans(input_lss, target_lss, *in->cache, hashdex, in->options, *in->stateless_relabeler, *out,
                                                                       in->shards_inner_series, in->shards_relabeled_series, in->def_timestamp);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans_from_cache(void* args, void* res) {
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
          auto& input_lss = std::get<entrypoint::head::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          out->ok = in->per_goroutine_relabeler->input_relabeling_with_stalenans_from_cache(input_lss, target_lss, *in->cache, hashdex, in->options, *out,
                                                                                            in->shards_inner_series, in->def_timestamp);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling(void* args, void* res) {
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
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          const entrypoint::head::ReallocationsDetector reallocation_detector(target_lss);
          in->per_goroutine_relabeler->input_transition_relabeling(target_lss, hashdex, *out, in->shards_inner_series);
          target_lss.build_deferred_indexes();
          out->target_lss_has_reallocations = reallocation_detector.has_reallocations();
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling_only_read(void* args, void* res) {
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
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);

          out->ok = in->per_goroutine_relabeler->input_transition_relabeling_only_read(target_lss, hashdex, *out, in->shards_inner_series);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_append_relabeler_series(void* args, void* res) {
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
    auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);
    const entrypoint::head::ReallocationsDetector reallocation_detector(lss);

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
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_goroutine_relabeler_track_stale_nans(void* args) {
  struct Arguments {
    SliceView<InnerSeries> inner_series;
    StaleNaNsStatePtr stale_nans_state;
    PromPP::Primitives::Timestamp default_timestamp;
  };

  const auto in = static_cast<Arguments*>(args);
  PerGoroutineRelabeler::track_stale_nans(in->inner_series, *in->stale_nans_state, in->default_timestamp);
}

extern "C" void prompp_remap_stale_nans_state(void* args) {
  struct Arguments {
    StaleNaNsStatePtr stale_nans_state;
    entrypoint::head::LsIdsSlicePtr dst_src_ls_ids_mapping;
  };

  const auto in = static_cast<Arguments*>(args);
  in->stale_nans_state->remap(*in->dst_src_ls_ids_mapping);
}
