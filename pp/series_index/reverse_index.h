#pragma once

#include <array>
#include <span>

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"

namespace series_index {

static constexpr uint32_t kOptimalPreAllocationElementsCount = 8;

template <template <class> class MemoryType = BareBones::MemoryWithItemCount>
class CompactSeriesIdSequence {
 public:
  enum class Type : uint8_t { kArray = 0, kSequence };

  using SeriesIdSequence = BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<
      BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124, MemoryType, kOptimalPreAllocationElementsCount>>>;

  static constexpr uint32_t kMaxElementsInArray = sizeof(SeriesIdSequence) / sizeof(typename SeriesIdSequence::value_type);
  using Array = std::array<typename SeriesIdSequence::value_type, kMaxElementsInArray>;
  using value_type = typename SeriesIdSequence::value_type;

  PROMPP_ALWAYS_INLINE explicit CompactSeriesIdSequence(Type type = Type::kArray) : type_(type) {
    if (type_ == Type::kSequence) {
      new (&sequence_impl_buffer_) SeriesIdSequence();
    }
  }

  PROMPP_ALWAYS_INLINE ~CompactSeriesIdSequence() {
    if (type_ == Type::kSequence) {
      sequence().~SeriesIdSequence();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return type_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return elements_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return elements_count_ == 0; }

  void push_back(uint32_t value) {
    if (type_ == Type::kArray) {
      if (elements_count_ < kMaxElementsInArray) {
        sequence_impl_buffer_[elements_count_] = value;
        ++elements_count_;
        return;
      }

      switch_to_sequence();
    }

    const_cast<SeriesIdSequence&>(sequence()).push_back(value);
    ++elements_count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    if (type_ == Type::kArray) {
      return 0;
    }

    return sequence().allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence& sequence() const noexcept {
    assert(type_ == Type::kSequence);
    return *reinterpret_cast<const SeriesIdSequence*>(sequence_impl_buffer_.data());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const typename SeriesIdSequence::value_type> array() const noexcept {
    assert(type_ == Type::kArray);
    return {(sequence_impl_buffer_.data()), elements_count_};
  }

  template <class Processor>
  PROMPP_ALWAYS_INLINE auto process_series(Processor&& processor) const noexcept {
    if (type_ == Type::kArray) {
      return processor(array());
    } else {
      return processor(sequence());
    }
  }

 private:
  alignas(alignof(SeriesIdSequence)) Array sequence_impl_buffer_;
  uint32_t elements_count_{};
  Type type_;

  PROMPP_ALWAYS_INLINE void switch_to_sequence() {
    Array buffer_copy = sequence_impl_buffer_;

    new (&sequence_impl_buffer_) SeriesIdSequence();
    type_ = Type::kSequence;

    std::ranges::copy(buffer_copy, std::back_inserter(const_cast<SeriesIdSequence&>(sequence())));
  }
};

}  // namespace series_index

template <template <class> class MemoryType>
struct BareBones::IsTriviallyReallocatable<series_index::CompactSeriesIdSequence<MemoryType>> : std::true_type {};  // namespace BareBones

namespace series_index {

template <template <class> class Vector = BareBones::Vector, template <class> class MemoryType = BareBones::MemoryWithItemCount>
class LabelReverseIndex {
 public:
  using SeriesIdSequence = CompactSeriesIdSequence<MemoryType>;

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

  [[nodiscard]] PROMPP_ALWAYS_INLINE const Vector<SeriesIdSequence>& series_by_value() const noexcept { return series_by_value_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return series_by_value_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return all_series_.allocated_memory() + series_by_value_.allocated_memory(); }

 private:
  SeriesIdSequence all_series_{SeriesIdSequence::Type::kSequence};
  Vector<SeriesIdSequence> series_by_value_;
};

}  // namespace series_index

template <template <class> class Vector, template <class> class MemoryType>
struct BareBones::IsTriviallyReallocatable<series_index::LabelReverseIndex<Vector, MemoryType>> : std::true_type {};  // namespace BareBones

namespace series_index {

template <template <class> class Vector = BareBones::Vector, template <class> class MemoryType = BareBones::MemoryWithItemCount>
class SeriesReverseIndex {
 public:
  using LabelReverseIndex = series_index::LabelReverseIndex<Vector, MemoryType>;
  using SeriesIdSequence = typename LabelReverseIndex::SeriesIdSequence;
  using SeriesIdSequenceVector = Vector<SeriesIdSequence>;

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

  [[nodiscard]] PROMPP_ALWAYS_INLINE const Vector<LabelReverseIndex>& labels_by_name() const noexcept { return labels_by_name_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t names_count() const noexcept { return labels_by_name_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t values_count(uint32_t label_name_id) const noexcept {
    return exists(label_name_id) ? labels_by_name_[label_name_id].count() : 0;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return labels_by_name_.allocated_memory(); }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { labels_by_name_.reserve(size); }

 private:
  Vector<LabelReverseIndex> labels_by_name_;
};

}  // namespace series_index
