#pragma once

#include "primitives/go_model.h"
#include "primitives/hash.h"
#include "prometheus/hashdex.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace PromPP::WAL::hashdex {

template <class Lss, class DataStorage>
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

    PROMPP(inline) Iterator(const Hashes* hashes, const Lss* lss, const DataStorage* data_storage)
        : hashes_(hashes), lss_(lss), data_storage_(data_storage), max_ls_id_(lss->next_item_index()) {}

    PROMPP(inline) const Iterator& operator*() const noexcept { return *this; }
    PROMPP(inline) const Iterator* operator->() const noexcept { return this; }

    [[nodiscard]] PROMPP(inline) size_t hash() const { return hashes_->operator[](ls_id_); }

    template <class Timeseries>
    PROMPP(inline) void read(Timeseries& timeseries) const {
      using enum series_data::chunk::DataChunk::Type;

      timeseries.label_set().append_sorted(lss_->operator[](ls_id_));
      series_data::Decoder::decode_series(*data_storage_, ls_id_, [&timeseries](const series_data::encoder::Sample& sample) PROMPP(lambda_inline) {
        timeseries.samples().emplace_back(sample.timestamp, sample.value);
        return true;
      });
    }

    PROMPP(inline) bool operator==(const IteratorSentinel&) const noexcept { return ls_id_ == max_ls_id_; }

    PROMPP(inline) Iterator& operator++() noexcept {
      ++ls_id_;
      return *this;
    }

    PROMPP(inline) Iterator operator++(int) noexcept {
      auto tmp = *this;
      ++(*this);
      return tmp;
    }

   private:
    const Hashes* hashes_;
    const Lss* lss_;
    const DataStorage* data_storage_;
    uint32_t ls_id_{};
    uint32_t max_ls_id_;
  };

  class Floats {
   public:
    PROMPP(inline) void reset(const Lss* lss, const DataStorage* data_storage) noexcept {
      lss_ = lss;
      data_storage_ = data_storage;

      hashes_.clear();
      hashes_.reserve(lss->next_item_index());
      for (uint32_t ls_id = 0, max_ls_id = lss->next_item_index(); ls_id < max_ls_id; ++ls_id) {
        hashes_.emplace_back(Primitives::hash::hash_of_label_set((*lss)[ls_id]));
      }
    }

    [[nodiscard]] PROMPP(inline) size_t size() const noexcept { return hashes_.size(); }
    [[nodiscard]] PROMPP(inline) Iterator begin() const noexcept { return Iterator{&hashes_, lss_, data_storage_}; }
    [[nodiscard]] static PROMPP(inline) IteratorSentinel end() noexcept { return {}; }

   private:
    Hashes hashes_;
    const Lss* lss_{};
    const DataStorage* data_storage_{};
  };

  PROMPP(inline) void presharding(const Lss* lss, DataStorage* data_storage) {
    series_data::Encoder encoder(*data_storage);
    series_data::OutdatedChunkMerger{encoder}.merge();

    floats_.reset(lss, data_storage);
  }

  [[nodiscard]] PROMPP(inline) const auto& floats() const noexcept { return floats_; }
  [[nodiscard]] static PROMPP(inline) auto metadata() noexcept {
    struct Stub {};
    return Stub{};
  }

 private:
  Floats floats_;
};

}  // namespace PromPP::WAL::hashdex
