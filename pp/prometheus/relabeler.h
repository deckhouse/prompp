#pragma once

#include <cstring>
#include <string>
#include <string_view>

#include <parallel_hashmap/phmap.h>
#include <roaring/roaring.hh>

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "hashdex.h"
#include "primitives/go_slice.h"
#include "primitives/labels_builder.h"
#include "primitives/sample.h"
#include "primitives/timeseries.h"
#include "stateless_relabeler.h"
#include "value.h"

namespace PromPP::Prometheus::Relabel {

// MetricLimits limits on label set and samples.
struct MetricLimits {
  size_t label_limit{0};
  size_t label_name_length_limit{0};
  size_t label_value_length_limit{0};
  size_t sample_limit{0};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool label_limit_exceeded(size_t labels_count) const { return label_limit > 0 && labels_count > label_limit; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool samples_limit_exceeded(size_t samples_count) const { return sample_limit > 0 && samples_count >= sample_limit; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool label_name_length_limit_exceeded(size_t label_name_length) const {
    return label_name_length_limit > 0 && label_name_length > label_name_length_limit;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool label_value_length_limit_exceeded(size_t label_value_length) const {
    return label_value_length_limit > 0 && label_value_length > label_value_length_limit;
  }
};

// hard_validate on empty, name label(__name__) mandatory, valid label name and value) validate label set.
template <class LabelsBuilder>
PROMPP_ALWAYS_INLINE void hard_validate(relabelStatus& rstatus, LabelsBuilder& builder, const MetricLimits* limits) {
  if (rstatus == rsDrop) {
    return;
  }

  // check on empty labels set
  if (builder.is_empty()) [[unlikely]] {
    rstatus = rsDrop;
    return;
  }

  // check on contains metric name labels set
  if (!builder.contains(kMetricLabelName)) [[unlikely]] {
    rstatus = rsInvalid;
    return;
  }

  // validate labels
  builder.range([&](const auto& lname, const auto& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (lname == kMetricLabelName) [[unlikely]] {
      if (!metric_name_value_is_valid(lvalue)) [[unlikely]] {
        rstatus = rsInvalid;
        return false;
      }

      return true;
    }

    if (!label_name_is_valid(lname) || !label_value_is_valid(lvalue)) [[unlikely]] {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
  if (rstatus == rsInvalid) [[unlikely]] {
    return;
  }

  if (limits == nullptr) {
    return;
  }

  // check limit len serie
  if (limits->label_limit_exceeded(builder.size())) {
    rstatus = rsInvalid;
    return;
  }

  if (limits->label_name_length_limit == 0 && limits->label_value_length_limit == 0) {
    return;
  }

  // check limit len label name and value
  builder.range([&](const auto& lname, auto& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (limits->label_name_length_limit_exceeded(lname.size())) {
      rstatus = rsInvalid;
      return false;
    }

    if (limits->label_value_length_limit_exceeded(lvalue.size())) {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
};

// InnerSerie - timeserie after relabeling.
//
// samples - incoming samples;
// ls_id   - relabeling ls id from lss;
#pragma pack(push, 1)
struct InnerSerie {
  Primitives::Sample sample;
  uint32_t ls_id;

  PROMPP_ALWAYS_INLINE bool operator==(const InnerSerie& rt) const noexcept = default;
};
#pragma pack(pop)

// InnerSeries - vector with relabeled result.
//
// size - number of timeseries processed;
// data - vector with timeseries;
class InnerSeries {
  size_t size_{0};
  BareBones::Vector<InnerSerie> data_;
  roaring::Roaring tracked_stale_nans_;

 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<InnerSerie>& data() const { return data_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  PROMPP_ALWAYS_INLINE void reserve(size_t n) { data_.reserve(n); }

  PROMPP_ALWAYS_INLINE void emplace_back(const Primitives::Sample& sample, uint32_t ls_id, bool track_stale_nans) {
    data_.emplace_back(sample, ls_id);

    if (track_stale_nans) [[likely]] {
      tracked_stale_nans_.add(ls_id);
    }

    ++size_;
  }

  PROMPP_ALWAYS_INLINE void emplace_back(auto const& samples, uint32_t ls_id, bool track_stale_nans) {
    data_.reserve_and_write(samples.size(), [&](InnerSerie* series_buffer, uint32_t series_size) {
      for (const auto& sample : samples) {
        std::construct_at(series_buffer, sample, ls_id);
        ++series_buffer;
      }
      return series_size;
    });
    size_ += samples.size();

    if (track_stale_nans) [[likely]] {
      tracked_stale_nans_.add(ls_id);
    }
  }

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    data_.clear();
    size_ = 0;
    tracked_stale_nans_ = roaring::Roaring{};
  }

  PROMPP_ALWAYS_INLINE roaring::Roaring& tracked_stale_nans() { return tracked_stale_nans_; }
};

// RelabeledSerie - element after relabeling with new ls(for next step).
//
// ls      - relabeling new label set;
// samples - incoming samples;
// hash    - hash sum from ls;
// ls_id   - incoming ls id from lss;
struct RelabeledSerie {
  Primitives::LabelSet ls;
  BareBones::Vector<Primitives::Sample> samples;
  size_t hash;
  uint32_t ls_id;
};

// RelabeledSeries - vector with relabeling elements.
//
// size - number of timeseries processed;
// data - vector with RelabelElement;
class RelabeledSeries {
  size_t size_{0};
  std::vector<RelabeledSerie> data_;
  roaring::Roaring tracked_stale_nans_;

 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE const std::vector<RelabeledSerie>& data() const { return data_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  template <class Samples>
  PROMPP_ALWAYS_INLINE void emplace_back(const Primitives::LabelSet& ls, Samples&& samples, const size_t hash, const uint32_t ls_id, bool track_stale_nans) {
    data_.emplace_back(ls, std::forward<Samples>(samples), hash, ls_id);
    ++size_;

    if (track_stale_nans) {
      tracked_stale_nans_.add(ls_id);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_stale_nan_tracked(uint32_t ls_id) const { return tracked_stale_nans_.contains(ls_id); }

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    data_.clear();
    size_ = 0;
    tracked_stale_nans_ = roaring::Roaring{};
  }
};

// CacheValue - value for cache map.
//
// ls_id    - relabeled ls id;
// shard_id - relabeled shard id;
struct PROMPP_ATTRIBUTE_PACKED CacheValue {
  uint32_t ls_id{};
  uint16_t shard_id{};
};

// IncomingAndRelabeledLsID - for update cache.
struct IncomingAndRelabeledLsID {
  uint32_t incoming_ls_id{};
  uint32_t relabeled_ls_id{};
};

// RelabelerStateUpdate - container for update states.
using RelabelerStateUpdate = Primitives::Go::Slice<IncomingAndRelabeledLsID>;

class NoOpStaleNaNsState {
 public:
  PROMPP_ALWAYS_INLINE static void add_input([[maybe_unused]] uint32_t id) {}
  PROMPP_ALWAYS_INLINE static void add_target([[maybe_unused]] uint32_t id) {}

  template <typename InputCallback, typename TargetCallback>
  PROMPP_ALWAYS_INLINE static void swap([[maybe_unused]] InputCallback input_fn, [[maybe_unused]] TargetCallback target_fn) {}
};

class StaleNaNsState {
 public:
  template <class Callback>
  PROMPP_ALWAYS_INLINE void swap(roaring::Roaring&& current, Callback callback) {
    previous_ -= current;
    for (uint32_t ls_id : previous_) {
      callback(ls_id);
    }
    previous_ = std::move(current);
  }

  template <class Container>
  void remap(const Container& dst_src_ls_ids_mapping) {
    roaring::Roaring new_state;

    uint32_t dst_ls_id = 0;
    for (auto src_ls_id : dst_src_ls_ids_mapping) {
      if (previous_.contains(src_ls_id)) {
        new_state.add(dst_ls_id);
      }

      ++dst_ls_id;
    }

    previous_ = std::move(new_state);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const roaring::Roaring& state() const noexcept { return previous_; }

 private:
  roaring::Roaring previous_;
};

// Cache stateless cache for relabeler.
class Cache {
  size_t cache_allocated_memory_{0};
  phmap::parallel_flat_hash_map<uint32_t, CacheValue, std::hash<uint32_t>, std::equal_to<>, BareBones::Allocator<std::pair<const uint32_t, CacheValue>>>
      cache_relabel_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, CacheValue>>{cache_allocated_memory_}};
  roaring::Roaring cache_keep_{};
  roaring::Roaring cache_drop_{};

 public:
  // allocated_memory return size of allocated memory for caches.
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return cache_allocated_memory_ + cache_keep_.getSizeInBytes() + cache_drop_.getSizeInBytes();
  }

  // add_drop add ls id to drop cache.
  PROMPP_ALWAYS_INLINE void add_drop(const uint32_t ls_id) { cache_drop_.add(ls_id); }

  // add_keep add ls id to keep cache.
  PROMPP_ALWAYS_INLINE void add_keep(const uint32_t ls_id) { cache_keep_.add(ls_id); }

  // add_relabel add ls id to relabel cache.
  PROMPP_ALWAYS_INLINE void add_relabel(const uint32_t ls_id, const uint32_t relabeled_ls_id, const uint16_t relabeled_shard_id) noexcept {
    cache_relabel_.emplace(ls_id, CacheValue{.ls_id = relabeled_ls_id, .shard_id = relabeled_shard_id});
  }

  // run optimization on bitset caches.
  PROMPP_ALWAYS_INLINE void optimize() {
    cache_keep_.runOptimize();
    cache_drop_.runOptimize();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double part_of_drops() const {
    if (cache_drop_.cardinality() == 0) {
      return 0;
    }

    return std::bit_cast<double>(cache_drop_.cardinality()) /
           std::bit_cast<double>(cache_drop_.cardinality() + cache_keep_.cardinality() + cache_relabel_.size());
  }

  struct CheckResult {
    enum Status : uint8_t {
      kNotFound = 0,
      kDrop = 1,
      kKeep = 2,
      kRelabel = 3,
    };
    Status status{Status::kNotFound};
    uint16_t shard_id{};  // used only for kRelabel status
    uint32_t ls_id{};
    uint32_t source_ls_id{};  // used only for kRelabel status
  };

  template <class InputLSS, class TargetLSS, class LabelSet>
  PROMPP_ALWAYS_INLINE CheckResult check(const InputLSS& input_lss, const TargetLSS& target_lss, LabelSet& label_set, size_t hash) {
    if (const auto ls_id = input_lss.find(label_set, hash); ls_id.has_value()) {
      if (auto res = check_input(ls_id.value()); res.status != CheckResult::kNotFound) {
        return res;
      }
    }
    if (const auto ls_id = target_lss.find(label_set, hash); ls_id.has_value()) {
      return check_target(ls_id.value());
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE CheckResult check_input(uint32_t ls_id) {
    if (cache_drop_.contains(ls_id)) {
      return {.status = CheckResult::Status::kDrop};
    }

    if (const auto it = cache_relabel_.find(ls_id); it != cache_relabel_.end()) {
      return {.status = CheckResult::Status::kRelabel, .shard_id = it->second.shard_id, .ls_id = it->second.ls_id, .source_ls_id = ls_id};
    }

    return {};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CheckResult check_target(uint32_t ls_id) const {
    if (cache_keep_.contains(ls_id)) {
      return {.status = CheckResult::Status::kKeep, .ls_id = ls_id};
    }

    return {};
  }

  // third stage
  // update add to cache relabled data.
  PROMPP_ALWAYS_INLINE void update(const RelabelerStateUpdate& relabeler_state_update, const uint16_t relabeled_shard_id) {
    for (const auto& update : relabeler_state_update) {
      add_relabel(update.incoming_ls_id, update.relabeled_ls_id, relabeled_shard_id);
    }
  }
};

struct RelabelerOptions {
  Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>> target_labels{};
  MetricLimits* metric_limits{nullptr};
  bool honor_labels{false};
  bool track_timestamps_staleness{false};
  bool honor_timestamps{false};
};

// PerShardRelabeler - relabeler for shard.
//
// buf_                 - stringstream for construct pattern part;
// builder_state_       - state of label set builder;
// timeseries_buf_      - buffer for read incoming timeseries;
// stateless_relabeler_ - shared stateless relabeler, pointer;
// shard_id_            - current shard id;
// log_shards_          - logarithm to the base 2 of total shards count;
class PerShardRelabeler {
  std::string buf_;
  Primitives::LabelsBuilder builder_;
  std::vector<Primitives::LabelView> external_labels_{};
  Primitives::TimeseriesSemiview timeseries_buf_;
  StatelessRelabeler* stateless_relabeler_;
  uint16_t number_of_shards_;
  uint16_t shard_id_;

 public:
  // PerShardRelabeler - constructor. Init only with pre-initialized LSS* and StatelessRelabeler*.
  PROMPP_ALWAYS_INLINE PerShardRelabeler(Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                         StatelessRelabeler* stateless_relabeler,
                                         const uint16_t number_of_shards,
                                         const uint16_t shard_id)
      : stateless_relabeler_(stateless_relabeler), number_of_shards_(number_of_shards), shard_id_(shard_id) {
    if (stateless_relabeler_ == nullptr) [[unlikely]] {
      throw BareBones::Exception(0xabd6db40882fd6aa, "stateless relabeler is null pointer");
    }

    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

 public:
  // update_relabeler_state - add to cache relabled data(third stage).
  PROMPP_ALWAYS_INLINE static void update_relabeler_state(Cache& cache, const RelabelerStateUpdate& relabeler_state_update, const uint16_t relabeled_shard_id) {
    for (const auto& update : relabeler_state_update) {
      cache.add_relabel(update.incoming_ls_id, update.relabeled_ls_id, relabeled_shard_id);
    }
  }

  // output_relabeling - relabeling output series(fourth stage).
  template <class LSS>
  PROMPP_ALWAYS_INLINE void output_relabeling(const LSS& lss,
                                              Cache& cache,
                                              RelabeledSeries* relabeled_series,
                                              Primitives::Go::SliceView<InnerSeries>& incoming_inner_series,
                                              Primitives::Go::SliceView<InnerSeries>& encoders_inner_series) {
    for (const auto& inner_series : incoming_inner_series) {
      if (inner_series.size() == 0) {
        continue;
      }

      std::ranges::for_each(inner_series.data(), [&](const InnerSerie& inner_serie) PROMPP_LAMBDA_INLINE {
        const auto res = cache.check_input(inner_serie.ls_id);
        if (res.status == Cache::CheckResult::kDrop) {
          return;
        }

        if (res.status == Cache::CheckResult::kRelabel) {
          encoders_inner_series[res.shard_id].emplace_back(inner_serie.sample, res.ls_id, false);
          return;
        }

        if (inner_serie.ls_id >= lss.next_item_index()) [[unlikely]] {
          throw BareBones::Exception(0x7763a97e1717e835, "ls_id out of range: %d next_item_index: %d shard_id: %d", inner_serie.ls_id, lss.next_item_index(),
                                     shard_id_);
        }
        builder_.reset(lss[inner_serie.ls_id]);
        process_external_labels(builder_, external_labels_);

        relabelStatus rstatus = stateless_relabeler_->relabeling_process(buf_, builder_);
        soft_validate(rstatus, builder_);
        if (rstatus == rsDrop) {
          cache.add_drop(inner_serie.ls_id);
          return;
        }

        const auto& new_label_set = builder_.label_set();
        relabeled_series->emplace_back(new_label_set, BareBones::Vector{inner_serie.sample}, hash_value(new_label_set), inner_serie.ls_id, false);
      });
    }

    cache.optimize();
  }

  // reset set new number_of_shards and external_labels.
  PROMPP_ALWAYS_INLINE void reset_to(const Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                     const uint16_t number_of_shards) {
    number_of_shards_ = number_of_shards;
    external_labels_.clear();
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }
};

//
// PerGoroutineRelabeler
//

// PerGoroutineRelabeler stateful relabeler for shard goroutines.
template <template <class> class SeriesContainer>
class PerGoroutineRelabeler {
  std::string buf_;
  Primitives::LabelsBuilder builder_;
  Primitives::TimeseriesSemiview timeseries_buf_;
  uint16_t number_of_shards_;
  uint16_t shard_id_;

 public:
  // PerShardRelabeler constructor.
  PROMPP_ALWAYS_INLINE PerGoroutineRelabeler(const uint16_t number_of_shards, const uint16_t shard_id)
      : number_of_shards_(number_of_shards), shard_id_(shard_id) {}

 private:
  PROMPP_ALWAYS_INLINE static size_t non_stale_nan_samples_count(const BareBones::Vector<Primitives::Sample>& samples) noexcept {
    return std::ranges::count_if(samples, [](const Primitives::Sample& sample) { return !is_stale_nan(sample.value()); });
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE bool input_relabeling_from_cache_internal(InputLSS& input_lss,
                                                                 TargetLSS& target_lss,
                                                                 Cache& cache,
                                                                 const Hashdex& hashdex,
                                                                 const RelabelerOptions& options,
                                                                 Stats& stats,
                                                                 SeriesContainer<InnerSeries>& shards_inner_series,
                                                                 Primitives::Timestamp def_timestamp) {
    bool result{true};
    size_t samples_count{};
    fill_inner_series(hashdex, hashdex.begin(), shards_inner_series, [&](auto& item) {
      const auto check_result = cache.check(input_lss, target_lss, timeseries_buf_.label_set(), item.hash());
      switch (check_result.status) {
        case Cache::CheckResult::kNotFound: {
          result = false;
          return false;
        };

        case Cache::CheckResult::kKeep: {
          auto& samples = timeseries_buf_.samples();
          const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
          shards_inner_series[shard_id_].emplace_back(samples, check_result.ls_id, options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
          break;
        }

        case Cache::CheckResult::kRelabel: {
          auto& samples = timeseries_buf_.samples();
          const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
          shards_inner_series[check_result.shard_id].emplace_back(samples, check_result.ls_id,
                                                                  options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
          break;
        }

        default: {
          return true;
        }
      }

      stats.samples_added += timeseries_buf_.samples().size();

      if (options.metric_limits != nullptr) {
        samples_count += non_stale_nan_samples_count(timeseries_buf_.samples());
        if (options.metric_limits->samples_limit_exceeded(samples_count)) {
          return false;
        }
      }

      return true;
    });

    return result;
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling_internal(InputLSS& input_lss,
                                                      TargetLSS& target_lss,
                                                      Cache& cache,
                                                      const Hashdex& hashdex,
                                                      const RelabelerOptions& options,
                                                      const StatelessRelabeler& stateless_relabeler,
                                                      Stats& stats,
                                                      SeriesContainer<InnerSeries>& shards_inner_series,
                                                      SeriesContainer<RelabeledSeries>& shards_relabeled_series,
                                                      Primitives::Timestamp def_timestamp) {
    size_t samples_count{};
    fill_inner_series(hashdex, skip_shard_inner_series(hashdex, shards_inner_series[shard_id_].size()), shards_inner_series, [&](auto& item) {
      const auto check_result = cache.check(input_lss, target_lss, timeseries_buf_.label_set(), item.hash());
      switch (check_result.status) {
        case Cache::CheckResult::kNotFound: {
          builder_.reset(timeseries_buf_.label_set());
          switch (relabel(options, stateless_relabeler, builder_)) {
            case rsDrop:
            case rsInvalid: {
              cache.add_drop(input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash()));
              ++stats.series_drop;
              return true;
            }

            case rsKeep: {
              auto ls_id = target_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              cache.add_keep(ls_id);
              auto& samples = timeseries_buf_.samples();
              const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
              shards_inner_series[shard_id_].emplace_back(samples, ls_id, options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
              ++stats.series_added;

              break;
            }

            case rsRelabel: {
              auto ls_id = input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              const auto& new_label_set = builder_.label_set();
              size_t new_hash = hash_value(new_label_set);
              const size_t new_shard_id = new_hash % number_of_shards_;
              auto& samples = timeseries_buf_.samples();
              const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
              shards_relabeled_series[new_shard_id].emplace_back(new_label_set, samples, new_hash, ls_id,
                                                                 options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
              ++stats.series_added;

              break;
            }
          }

          break;
        }

        case Cache::CheckResult::kKeep: {
          auto& samples = timeseries_buf_.samples();
          const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
          shards_inner_series[shard_id_].emplace_back(samples, check_result.ls_id, options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
          break;
        }

        case Cache::CheckResult::kRelabel: {
          auto& samples = timeseries_buf_.samples();
          const bool all_samples_reseted_to_scrape_ts = resolve_timestamps(def_timestamp, samples, options);
          shards_inner_series[check_result.shard_id].emplace_back(samples, check_result.ls_id,
                                                                  options.track_timestamps_staleness || all_samples_reseted_to_scrape_ts);
          break;
        }

        default: {
          break;
        }
      }

      stats.samples_added += timeseries_buf_.samples().size();

      if (options.metric_limits != nullptr) {
        samples_count += non_stale_nan_samples_count(timeseries_buf_.samples());
        if (options.metric_limits->samples_limit_exceeded(samples_count)) {
          return false;
        }
      }

      return true;
    });

    cache.optimize();
  }

  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabel(const RelabelerOptions& options, const StatelessRelabeler& stateless_relabeler, LabelsBuilder& builder) {
    const bool changed = inject_target_labels(builder, options);

    relabelStatus rstatus = stateless_relabeler.relabeling_process(buf_, builder);
    hard_validate(rstatus, builder, options.metric_limits);
    if (changed && rstatus == rsKeep) {
      rstatus = rsRelabel;
    }

    return rstatus;
  }

  PROMPP_ALWAYS_INLINE static bool resolve_timestamps(Primitives::Timestamp def_timestamp,
                                                      BareBones::Vector<Primitives::Sample>& samples,
                                                      const RelabelerOptions& options) {
    // skip resolve without stalenans
    if (def_timestamp == Primitives::kNullTimestamp) {
      return false;
    }

    bool track_staleness{true};
    for (auto& sample : samples) {
      // replace null timestamp on def timestamp
      if (sample.timestamp() == Primitives::kNullTimestamp) {
        sample.timestamp() = def_timestamp;
        continue;
      }

      // replace incoming timestamp on def timestamp
      if (!options.honor_timestamps) {
        sample.timestamp() = def_timestamp;
        continue;
      }

      track_staleness = false;
    }

    return track_staleness;
  }

  template <hashdex::HashdexInterface Hashdex>
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto skip_shard_inner_series(const Hashdex& hashdex, size_t i) {
    auto it = hashdex.begin();
    for (; it != hashdex.end() && i > 0; ++it) {
      if ((it->hash() % number_of_shards_) != shard_id_) {
        continue;
      }
      --i;
    }

    return it;
  }

 public:
  // inject_target_labels add labels from target to builder.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE bool inject_target_labels(LabelsBuilder& target_builder, const RelabelerOptions& options) {
    if (options.target_labels.empty()) {
      return false;
    }

    bool changed{false};

    if (options.honor_labels) {
      for (const auto& [lname, lvalue] : options.target_labels) {
        if (target_builder.contains(static_cast<std::string_view>(lname))) [[unlikely]] {
          continue;
        }

        target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
        changed = true;
      }

      return changed;
    }

    std::vector<Primitives::Label> conflicting_exposed_labels;
    for (const auto& [lname, lvalue] : options.target_labels) {
      Primitives::Label existing_label = target_builder.extract(static_cast<std::string_view>(lname));
      if (!existing_label.second.empty()) [[likely]] {
        conflicting_exposed_labels.emplace_back(std::move(existing_label));
      }

      // It is now safe to set the target label.
      target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
      changed = true;
    }

    // resolve conflict
    if (!conflicting_exposed_labels.empty()) {
      resolve_conflicting_exposed_labels(target_builder, conflicting_exposed_labels);
    }

    return changed;
  }

  // resolve_conflicting_exposed_labels add prefix to conflicting label name.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE void resolve_conflicting_exposed_labels(LabelsBuilder& builder, std::vector<Primitives::Label>& conflicting_exposed_labels) {
    std::stable_sort(conflicting_exposed_labels.begin(), conflicting_exposed_labels.end(),
                     [](const Primitives::LabelView& a, const Primitives::LabelView& b) { return a.first.size() < b.first.size(); });

    for (auto& [ln, lv] : conflicting_exposed_labels) {
      while (true) {
        ln.insert(0, "exported_");
        if (builder.get(ln).empty()) {
          builder.set(ln, lv);
          break;
        }
      }
    }
  }

  // first stage
  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling(InputLSS& input_lss,
                                             TargetLSS& target_lss,
                                             Cache& cache,
                                             const Hashdex& hashdex,
                                             const RelabelerOptions& options,
                                             const StatelessRelabeler& stateless_relabeler,
                                             Stats& stats,
                                             SeriesContainer<InnerSeries>& shards_inner_series,
                                             SeriesContainer<RelabeledSeries>& shards_relabeled_series) {
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, options, stateless_relabeler, stats, shards_inner_series, shards_relabeled_series,
                              Primitives::kNullTimestamp);
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE bool input_relabeling_from_cache(InputLSS& input_lss,
                                                        TargetLSS& target_lss,
                                                        Cache& cache,
                                                        const Hashdex& hashdex,
                                                        const RelabelerOptions& options,
                                                        Stats& stats,
                                                        SeriesContainer<InnerSeries>& shards_inner_series) {
    return input_relabeling_from_cache_internal(input_lss, target_lss, cache, hashdex, options, stats, shards_inner_series, Primitives::kNullTimestamp);
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_relabeling_with_stalenans(InputLSS& input_lss,
                                                            TargetLSS& target_lss,
                                                            Cache& cache,
                                                            const Hashdex& hashdex,
                                                            const RelabelerOptions& options,
                                                            const StatelessRelabeler& stateless_relabeler,
                                                            Stats& stats,
                                                            SeriesContainer<InnerSeries>& shards_inner_series,
                                                            SeriesContainer<RelabeledSeries>& shards_relabeled_series,
                                                            Primitives::Timestamp def_timestamp) {
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, options, stateless_relabeler, stats, shards_inner_series, shards_relabeled_series,
                              def_timestamp);
  }

  template <class InputLSS, class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE bool input_relabeling_with_stalenans_from_cache(InputLSS& input_lss,
                                                                       TargetLSS& target_lss,
                                                                       Cache& cache,
                                                                       const Hashdex& hashdex,
                                                                       const RelabelerOptions& options,
                                                                       Stats& stats,
                                                                       SeriesContainer<InnerSeries>& shards_inner_series,
                                                                       Primitives::Timestamp def_timestamp) {
    return input_relabeling_from_cache_internal(input_lss, target_lss, cache, hashdex, options, stats, shards_inner_series, def_timestamp);
  }

  // input_transition_relabeling transparent relabeling.
  template <class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE void input_transition_relabeling(TargetLSS& target_lss,
                                                        const Hashdex& hashdex,
                                                        Stats& stats,
                                                        SeriesContainer<InnerSeries>& shards_inner_series) {
    fill_inner_series(hashdex, skip_shard_inner_series(hashdex, shards_inner_series[shard_id_].size()), shards_inner_series, [&](auto& item) {
      const auto previous_size = target_lss.items_count();
      auto ls_id = target_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
      shards_inner_series[shard_id_].emplace_back(timeseries_buf_.samples(), ls_id, false);

      if (target_lss.items_count() > previous_size) {
        ++stats.series_added;
      }
      stats.samples_added += timeseries_buf_.samples().size();
      return true;
    });
  }

  // input_transition_relabeling_from_cache transparent relabeling with only reading from the lss.
  template <class TargetLSS, hashdex::HashdexInterface Hashdex, class Stats>
  PROMPP_ALWAYS_INLINE bool input_transition_relabeling_only_read(TargetLSS& target_lss,
                                                                  const Hashdex& hashdex,
                                                                  Stats& stats,
                                                                  SeriesContainer<InnerSeries>& shards_inner_series) {
    bool result = true;
    fill_inner_series(hashdex, hashdex.begin(), shards_inner_series, [&](auto& item) {
      if (auto ls_id = target_lss.find(timeseries_buf_.label_set(), item.hash()); ls_id.has_value()) {
        shards_inner_series[shard_id_].emplace_back(timeseries_buf_.samples(), *ls_id, false);
        stats.samples_added += timeseries_buf_.samples().size();
        return true;
      }

      result = false;
      return false;
    });

    return result;
  }

  // second stage
  // append_relabeler_series add relabeled ls to lss, add to result and add to cache update.
  template <class LSS>
  PROMPP_ALWAYS_INLINE static void append_relabeler_series(LSS& target_lss,
                                                           InnerSeries& inner_series,
                                                           const RelabeledSeries& relabeled_series,
                                                           RelabelerStateUpdate& relabeler_state_update) {
    relabeler_state_update.reserve(relabeler_state_update.size() + relabeled_series.size());
    inner_series.reserve(inner_series.size() + relabeled_series.size());
    if constexpr (BareBones::concepts::has_reserve<LSS>) {
      target_lss.reserve(target_lss.items_count() + relabeled_series.size());
    }

    for (const auto& relabeled_serie : relabeled_series.data()) {
      uint32_t ls_id = target_lss.find_or_emplace(relabeled_serie.ls, relabeled_serie.hash);
      inner_series.emplace_back(relabeled_serie.samples, ls_id, relabeled_series.is_stale_nan_tracked(relabeled_serie.ls_id));
      relabeler_state_update.emplace_back(relabeled_serie.ls_id, ls_id);
    }
  }

  template <hashdex::HashdexInterface Hashdex, class Handler>
  void fill_inner_series(const Hashdex& hashdex, auto hashdex_it, SeriesContainer<InnerSeries>& shards_inner_series, Handler handler) {
    assert(number_of_shards_ > 0);

    const size_t n = std::min(static_cast<size_t>(hashdex.size()), static_cast<size_t>((hashdex.size() * 1.1) / number_of_shards_));
    for (uint16_t i = 0; i < number_of_shards_; ++i) {
      shards_inner_series[i].reserve(n);
    }

    for (; hashdex_it != hashdex.end(); ++hashdex_it) {
      if ((hashdex_it->hash() % number_of_shards_) != shard_id_) {
        continue;
      }

      timeseries_buf_.clear();
      hashdex_it->read(timeseries_buf_);

      if (!handler(*hashdex_it)) {
        break;
      }
    }
  }

  static void track_stale_nans(SeriesContainer<InnerSeries>& inner_series, StaleNaNsState& state, Primitives::Timestamp def_timestamp) {
    roaring::Roaring current_state;

    for (auto& series : inner_series) {
      if (series.size() > 0) {
        current_state |= series.tracked_stale_nans();
      }
    }

    const Primitives::Sample sample{def_timestamp, kStaleNan};
    state.swap(std::move(current_state), [&](uint32_t ls_id) { inner_series[0].emplace_back(sample, ls_id, false); });
  }
};

}  // namespace PromPP::Prometheus::Relabel
