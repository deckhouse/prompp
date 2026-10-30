#pragma once

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"

namespace series_index {

static constexpr uint32_t kOptimalPreAllocationElementsCount = 8;

template <class T>
using SharedMemory = BareBones::SharedMemory<T, BareBones::SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

using PostingStore = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124, SharedMemory, kOptimalPreAllocationElementsCount>;
using PostingSequence = BareBones::RleSequence<BareBones::coding::Delta, PostingStore>;

class SeriesIdSequenceSnapshot;

class SeriesIdSequence {
  friend class SeriesIdSequenceSnapshot;
  [[no_unique_address]] PostingSequence sequence_{};
  uint32_t count_{};

 public:
  using value_type = uint32_t;
  using Iterator = PostingSequence::Iterator;
  using IteratorSentinel = PostingSequence::IteratorSentinel;
  SeriesIdSequence() = default;
  SeriesIdSequence(SeriesIdSequence&& source) noexcept : sequence_(std::move(source.sequence_)), count_(std::exchange(source.count_, 0)) {}
  SeriesIdSequence& operator=(SeriesIdSequence&& source) noexcept {
    if (this != &source) {
      sequence_ = std::move(source.sequence_);
      count_ = std::exchange(source.count_, 0);
    }
    return *this;
  }
  // Cardinality must fit uint32_t.
  void push_back(uint32_t value) noexcept {
    assert(count_ != std::numeric_limits<uint32_t>::max());
    sequence_.push_back(value);
    ++count_;
  }
  void clear() noexcept {
    sequence_.clear();
    count_ = 0;
  }
  [[nodiscard]] uint32_t count() const noexcept { return count_; }
  [[nodiscard]] bool empty() const noexcept { return count_ == 0; }
  [[nodiscard]] auto view() const noexcept { return sequence_.view(); }
  [[nodiscard]] auto progressions() const noexcept { return sequence_.progressions(); }
  [[nodiscard]] Iterator begin() const noexcept { return sequence_.begin(); }
  [[nodiscard]] static IteratorSentinel end() noexcept { return {}; }
  [[nodiscard]] size_t allocated_memory() const noexcept { return sequence_.allocated_memory(); }
};

// Iterators borrow this snapshot. Capture and traversal require writer exclusion.
// Moving leaves the source empty.
class SeriesIdSequenceSnapshot {
  [[no_unique_address]] PostingSequence::Snapshot sequence_{};
  uint32_t count_{};

 public:
  using value_type = uint32_t;
  using Iterator = PostingSequence::Iterator;
  SeriesIdSequenceSnapshot() = default;
  SeriesIdSequenceSnapshot(const SeriesIdSequenceSnapshot&) = default;
  SeriesIdSequenceSnapshot& operator=(const SeriesIdSequenceSnapshot&) = default;
  SeriesIdSequenceSnapshot(SeriesIdSequenceSnapshot&& other) noexcept : sequence_(std::move(other.sequence_)), count_(std::exchange(other.count_, 0)) {}
  SeriesIdSequenceSnapshot& operator=(SeriesIdSequenceSnapshot&& other) noexcept {
    if (this != &other) {
      sequence_ = std::move(other.sequence_);
      count_ = std::exchange(other.count_, 0);
    }
    return *this;
  }
  explicit SeriesIdSequenceSnapshot(const SeriesIdSequence& source) : sequence_(source.sequence_.snapshot()), count_(source.count_) {}
  SeriesIdSequenceSnapshot& operator=(const SeriesIdSequence& source) noexcept {
    sequence_ = source.sequence_.snapshot();
    count_ = source.count_;
    return *this;
  }
  [[nodiscard]] uint32_t count() const noexcept { return count_; }
  [[nodiscard]] bool empty() const noexcept { return count_ == 0; }
  [[nodiscard]] auto view() const noexcept { return sequence_.view(); }
  [[nodiscard]] auto progressions() const noexcept { return sequence_.progressions(); }
  [[nodiscard]] auto begin() const noexcept { return sequence_.begin(); }
  [[nodiscard]] static PostingSequence::IteratorSentinel end() noexcept { return {}; }
};

}  // namespace series_index

// Neither type stores pointers into its own object.
template <>
struct BareBones::IsTriviallyReallocatable<series_index::SeriesIdSequence> : std::true_type {};
template <>
struct BareBones::IsTriviallyReallocatable<series_index::SeriesIdSequenceSnapshot> : std::true_type {};

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
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence& get_existing(uint32_t label_value_id) const noexcept { return series_by_value_[label_value_id]; }
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
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesIdSequence& get_existing(uint32_t label_name_id) const noexcept {
    return *labels_by_name_[label_name_id].get_all();
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
