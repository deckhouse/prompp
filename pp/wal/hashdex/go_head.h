#pragma once

#include "primitives/go_model.h"
#include "primitives/hash.h"
#include "prometheus/hashdex.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace PromPP::WAL::hashdex {

template <class Lss>
class GoHead : public Prometheus::hashdex::Abstract {
 public:
  using Hashes = BareBones::Vector<size_t>;

  class IteratorSentinel {};

  class Iterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = Iterator;
    using difference_type = ptrdiff_t;
    using pointer = Iterator*;
    using reference = Iterator&;

    PROMPP_ALWAYS_INLINE Iterator(const Hashes* hashes, const Lss* lss, const series_data::DataStorage* data_storage)
        : hashes_(hashes), lss_(lss), data_storage_(data_storage), max_ls_id_(lss->size()) {}

    PROMPP_ALWAYS_INLINE const Iterator& operator*() const noexcept { return *this; }
    PROMPP_ALWAYS_INLINE const Iterator* operator->() const noexcept { return this; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hashes_->operator[](ls_id_); }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      using enum series_data::chunk::DataChunk::Type;

      timeseries.label_set().append_sorted(lss_->operator[](ls_id_));
      series_data::Decoder::decode_series(*data_storage_, ls_id_, [&timeseries](const series_data::encoder::Sample& sample) PROMPP_LAMBDA_INLINE {
        timeseries.samples().emplace_back(sample.timestamp, sample.value);
        return true;
      });
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return ls_id_ == max_ls_id_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      ++ls_id_;
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      auto tmp = *this;
      ++(*this);
      return tmp;
    }

   private:
    const Hashes* hashes_;
    const Lss* lss_;
    const series_data::DataStorage* data_storage_;
    uint32_t ls_id_{};
    uint32_t max_ls_id_;
  };

  class Metrics {
   public:
    PROMPP_ALWAYS_INLINE void reset(const Lss* lss, const series_data::DataStorage* data_storage) noexcept {
      lss_ = lss;
      data_storage_ = data_storage;

      hashes_.clear();
      hashes_.reserve(lss->size());
      for (const auto& label_set : *lss) {
        hashes_.emplace_back(Primitives::hash::hash_of_label_set(label_set));
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return hashes_.size(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator{&hashes_, lss_, data_storage_}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    Hashes hashes_;
    const Lss* lss_{};
    const series_data::DataStorage* data_storage_{};
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return metrics_.size(); }

  PROMPP_ALWAYS_INLINE void presharding(const Lss* lss, series_data::DataStorage* data_storage) {
    series_data::Encoder encoder(*data_storage);
    series_data::OutdatedChunkMerger{encoder}.merge();

    metrics_.reset(lss, data_storage);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return metrics_.begin(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return metrics_.end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& metrics() const noexcept { return metrics_; }
  [[nodiscard]] static PROMPP_ALWAYS_INLINE auto metadata() noexcept {
    struct Stub {};
    return Stub{};
  }

 private:
  Metrics metrics_;
};

}  // namespace PromPP::WAL::hashdex