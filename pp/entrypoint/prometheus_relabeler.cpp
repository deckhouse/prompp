#include "prometheus_relabeler.h"

#include "exception.hpp"
#include "hashdex.hpp"
#include "head/lss.h"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

using entrypoint::head::LssVariantPtr;

//
// StatelessRelabeler
//

using Cache = PromPP::Prometheus::Relabel::Cache;
using CachePtr = std::unique_ptr<Cache>;

using StatelessRelabeler = PromPP::Prometheus::Relabel::StatelessRelabeler;
using StatelessRelabelerPtr = std::unique_ptr<StatelessRelabeler>;

extern "C" void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
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
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
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
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  auto* in = static_cast<Arguments*>(args);
  new (in->inner_series) PromPP::Prometheus::Relabel::InnerSeries();
}

extern "C" void prompp_prometheus_inner_series_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  static_cast<Arguments*>(args)->inner_series->~InnerSeries();
}

//
// RelabeledSeries
//

extern "C" void prompp_prometheus_relabeled_series_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
  };

  auto* in = static_cast<Arguments*>(args);
  new (in->relabeled_series) PromPP::Prometheus::Relabel::RelabeledSeries();
}

extern "C" void prompp_prometheus_relabeled_series_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
  };

  static_cast<Arguments*>(args)->relabeled_series->~RelabeledSeries();
}

//
// RelabelerStateUpdate
//

extern "C" void prompp_prometheus_relabeler_state_update_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
  };

  auto* in = static_cast<Arguments*>(args);

  new (in->relabeler_state_update) PromPP::Prometheus::Relabel::RelabelerStateUpdate();
}

extern "C" void prompp_prometheus_relabeler_state_update_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
  };

  static_cast<Arguments*>(args)->relabeler_state_update->~vector();
}

//
// PerShardRelabeler
//

using PerShardRelabeler = PromPP::Prometheus::Relabel::PerShardRelabeler;
using PerShardRelabelerPtr = std::unique_ptr<PerShardRelabeler>;

extern "C" void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
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

extern "C" void prompp_prometheus_per_shard_relabeler_cache_allocated_memory(void* args, void* res) {
  struct Arguments {
    PerShardRelabelerPtr per_shard_relabeler;
  };
  struct Result {
    size_t allocated_memory;
  };

  const auto* in = static_cast<Arguments*>(args);
  new (res) Result{.allocated_memory = in->per_shard_relabeler->cache_allocated_memory()};
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerShardRelabelerPtr per_shard_relabeler;
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

          entrypoint::head::lss_memory::has_reallocations = false;
          in->per_shard_relabeler->input_relabeling(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series,
                                                    in->shards_relabeled_series);
          std::vector<uint32_t> ids;
          target_lss.sort_series_ids(ids);
          out->target_lss_has_reallocations = entrypoint::head::lss_memory::has_reallocations;
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

using StaleNaNsState = PromPP::Prometheus::Relabel::StaleNaNsState;
using StaleNaNsStatePtr = std::unique_ptr<StaleNaNsState>;

extern "C" void prompp_prometheus_relabel_stalenans_state_ctor(void* res) {
  struct Result {
    StaleNaNsStatePtr state;
  };

  new (res) Result{.state = std::make_unique<StaleNaNsState>()};
}

extern "C" void prompp_prometheus_relabel_stalenans_state_dtor(void* args) {
  struct Arguments {
    StaleNaNsStatePtr state;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_prometheus_relabel_stalenans_state_reset(void* args) {
  struct Arguments {
    StaleNaNsStatePtr state;
  };

  static_cast<Arguments*>(args)->state->reset();
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PerShardRelabelerPtr per_shard_relabeler;
    HashdexVariant* hashdex;
    CachePtr cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
    StaleNaNsStatePtr state;
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

          entrypoint::head::lss_memory::has_reallocations = false;
          in->per_shard_relabeler->input_relabeling_with_stalenans(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series,
                                                                   in->shards_relabeled_series, *in->state, in->def_timestamp);
          std::vector<uint32_t> ids;
          target_lss.sort_series_ids(ids);
          out->target_lss_has_reallocations = entrypoint::head::lss_memory::has_reallocations;
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_collect_stalenans(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PerShardRelabelerPtr per_shard_relabeler;
    CachePtr cache;
    StaleNaNsStatePtr state;
    PromPP::Primitives::Timestamp stale_ts;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto in = static_cast<Arguments*>(args);

  try {
    in->per_shard_relabeler->input_collect_stalenans(*in->cache, in->shards_inner_series, *in->state, in->stale_ts);
  } catch (...) {
    const auto out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_append_relabeler_series(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PerShardRelabelerPtr per_shard_relabeler;
    LssVariantPtr lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
    bool target_lss_has_reallocations{};
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);

    entrypoint::head::lss_memory::has_reallocations = false;
    in->per_shard_relabeler->append_relabeler_series(lss, in->inner_series, in->relabeled_series, in->relabeler_state_update);
    std::vector<uint32_t> ids;
    lss.sort_series_ids(ids);
    out->target_lss_has_reallocations = entrypoint::head::lss_memory::has_reallocations;
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_update_relabeler_state(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PerShardRelabelerPtr per_shard_relabeler;
    CachePtr cache;
    uint16_t relabeled_shard_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);

  try {
    in->per_shard_relabeler->update_relabeler_state(*in->cache, in->relabeler_state_update, in->relabeled_shard_id);
  } catch (...) {
    auto* out = new (res) Result();
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> encoders_inner_series;
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
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
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

extern "C" void prompp_prometheus_cache_reset_to(void* args) {
  struct Arguments {
    CachePtr cache;
  };

  static_cast<Arguments*>(args)->cache->reset();
}
