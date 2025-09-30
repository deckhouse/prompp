#pragma once

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"

namespace series_index {

static constexpr uint32_t kOptimalPreAllocationElementsCount = 8;

template <class T>
using SharedMemory = BareBones::SharedMemory<T, BareBones::DefaultReallocator>;

class DeltaRLE {
 public:
  using DataSequence = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124, SharedMemory, kOptimalPreAllocationElementsCount>;
  using BaseDeltaRLE = BareBones::Encoding::DeltaRLE<DataSequence>;

  class Encoder : public BaseDeltaRLE::Encoder {
   public:
    using value_type = DataSequence::value_type;

    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      BaseDeltaRLE::Encoder::encode(val, i);
      ++count_;
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept {
      BaseDeltaRLE::Encoder::clear();
      count_ = 0;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type count() const noexcept { return count_; }

   private:
    value_type count_{};
  };

  using Decoder = BaseDeltaRLE::Decoder;
};

class SeriesIdSequence : public BareBones::EncodedSequence<DeltaRLE> {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return encoder_.count(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t empty() const noexcept { return encoder_.count() == 0; }
};

class SeriesIdSequenceSnapshot {
 public:
  using value_type = DeltaRLE::DataSequence::value_type;

  SeriesIdSequenceSnapshot() = default;
  explicit SeriesIdSequenceSnapshot(const SeriesIdSequence& sequence) : encoder_(sequence.encoder()), memory_(sequence.data().buffer().ptr()) {}

  SeriesIdSequenceSnapshot& operator=(const SeriesIdSequence& sequence) noexcept {
    encoder_ = sequence.encoder();
    memory_ = sequence.data().buffer().ptr();
    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return encoder_.count(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t empty() const noexcept { return encoder_.count() == 0; }

  class Iterator {
   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = SeriesIdSequenceSnapshot::value_type;
    using difference_type = std::ptrdiff_t;

    Iterator(const SharedMemory<uint8_t>::SharedPtr& memory, const DeltaRLE::Encoder& encoder)
        : iterator_(DeltaRLE::DataSequence::decode_iterator(memory.get(), memory.constructed_item_count()), DeltaRLE::DataSequence::end(), &encoder),
          count_(encoder.count()) {}

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      ++iterator_;
      --count_;
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const SeriesIdSequence::IteratorSentinel&) const noexcept { return count_ == 0; }
    PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return *iterator_; }

   private:
    SeriesIdSequence::Iterator iterator_;
    uint32_t count_{};
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return {memory_, encoder_}; }
  static PROMPP_ALWAYS_INLINE SeriesIdSequence::IteratorSentinel end() noexcept { return {}; }

 private:
  DeltaRLE::Encoder encoder_;
  SharedMemory<uint8_t>::SharedPtr memory_;
};

}  // namespace series_index

template <>
struct BareBones::IsTriviallyReallocatable<series_index::SeriesIdSequence> : std::true_type {};  // namespace BareBones

template <>
struct BareBones::IsTriviallyReallocatable<series_index::SeriesIdSequenceSnapshot> : std::true_type {};  // namespace BareBones

namespace series_index {

class LabelReverseIndex {
 public:
  PROMPP_ALWAYS_INLINE void add(uint32_t label_value_id, uint32_t series_id) {
    if (!exists(label_value_id)) {
      series_by_value_.resize(label_value_id + 1);
    }

    series_by_value_[label_value_id].push_back(series_id);
    all_series_.push_back(series_id);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool exists(uint32_t label_value_id) const noexcept { return label_value_id < series_by_value_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence* get(uint32_t label_value_id) const noexcept {
    return exists(label_value_id) ? &series_by_value_[label_value_id] : nullptr;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence* get_all() const noexcept { return &all_series_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<SeriesIdSequence>& series_by_value() const noexcept { return series_by_value_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return series_by_value_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return all_series_.allocated_memory() + series_by_value_.allocated_memory(); }

 private:
  SeriesIdSequence all_series_{};
  BareBones::Vector<SeriesIdSequence> series_by_value_;
};

}  // namespace series_index

template <>
struct BareBones::IsTriviallyReallocatable<series_index::LabelReverseIndex> : std::true_type {};  // namespace BareBones

namespace series_index {

class SeriesReverseIndex {
 public:
  template <class Label>
  PROMPP_ALWAYS_INLINE void add(const Label& label, uint32_t series_id) {
    if (!exists(label.name_id())) {
      labels_by_name_.resize(label.name_id() + 1);
    }

    labels_by_name_[label.name_id()].add(label.value_id(), series_id);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool exists(uint32_t label_name_id) const noexcept { return label_name_id < labels_by_name_.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence* get(uint32_t label_name_id) const {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get_all() : nullptr;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence* get(uint32_t label_name_id, uint32_t label_value_id) const {
    return exists(label_name_id) ? labels_by_name_[label_name_id].get(label_value_id) : nullptr;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<LabelReverseIndex>& labels_by_name() const noexcept { return labels_by_name_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t names_count() const noexcept { return labels_by_name_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t values_count(uint32_t label_name_id) const noexcept {
    return exists(label_name_id) ? labels_by_name_[label_name_id].count() : 0;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return labels_by_name_.allocated_memory(); }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { labels_by_name_.reserve(size); }

 private:
  BareBones::Vector<LabelReverseIndex> labels_by_name_;
};

}  // namespace series_index
