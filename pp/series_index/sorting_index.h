#pragma once

#include "bare_bones/preprocess.h"

namespace series_index {

template <template <class> class Vector>
struct SortingIndex {
  static constexpr bool kIsReadOnly = BareBones::IsSharedSpan<Vector<uint32_t>>::value;

  SortingIndex() = default;
  template <template <class> class OtherVector>
    requires(kIsReadOnly)
  explicit SortingIndex(const SortingIndex<OtherVector>& other) : index(other.index) {}

  PROMPP_ALWAYS_INLINE void clear() noexcept
    requires(!kIsReadOnly)
  {
    index = Vector<uint32_t>{};
  }

  template <class Iterator>
  PROMPP_ALWAYS_INLINE void sort(Iterator begin, Iterator end) const noexcept {
    std::sort(begin, end, [this](uint32_t a, uint32_t b) PROMPP_LAMBDA_INLINE { return index[a] < index[b]; });
  }

  template <class Container>
  PROMPP_ALWAYS_INLINE void sort(Container& container) const noexcept {
    sort(container.begin(), container.end());
  }

  PROMPP_ALWAYS_INLINE auto get_comparator() const noexcept {
    return [this](uint32_t a, uint32_t b) PROMPP_LAMBDA_INLINE { return index[a] < index[b]; };
  }

  Vector<uint32_t> index;
};

template <class Set, template <class> class Vector, uint32_t kMaxIndexValue = std::numeric_limits<uint32_t>::max()>
class SortingIndexBuilder {
 public:
  using Index = SortingIndex<Vector>;
  using IndexValueType = uint32_t;

  explicit SortingIndexBuilder(const Set& ls_id_set) : ls_id_set_(ls_id_set) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return index_.index.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return index_.index.allocated_memory(); }

  PROMPP_ALWAYS_INLINE void build() {
    if (empty()) {
      rebuild();
    }
  }

  PROMPP_ALWAYS_INLINE void update(typename Set::const_iterator ls_id_iterator) {
    if (empty()) {
      return;
    }

    const uint64_t ls_id = *ls_id_iterator;
    max_id_ = std::max<uint64_t>(max_id_, ls_id);

    const uint64_t previous = get_previous(ls_id_iterator);
    const uint64_t next = get_next(ls_id_iterator);
    if (uint32_t value = (previous + next) / 2; value > previous) [[likely]] {
      index_.index.resize(max_id_ + 1);
      index_.index[ls_id] = value;
    } else {
      // If we can't insert item we don't need to rebuild index, because it's very expensive operation for CPU.
      // Index will be built on demand in sort method
      index_.clear();
    }
  }

  template <class Iterator>
  PROMPP_ALWAYS_INLINE void sort(Iterator begin, Iterator end) noexcept {
    build();
    index_.sort(begin, end);
  }

  PROMPP_ALWAYS_INLINE const Index& index() const noexcept { return index_; }

 private:
  const Set& ls_id_set_;
  Index index_;
  IndexValueType max_id_{0};

  void rebuild() {
    max_id_ = *std::max_element(ls_id_set_.begin(), ls_id_set_.end());
    index_.index.resize(max_id_ + 1);

    const uint32_t step = kMaxIndexValue / (max_id_ + 2);
    uint32_t index_value = 0;
    for (auto ls_id : ls_id_set_) {
      index_value += step;
      index_.index[ls_id] = index_value;
    }
  }

  PROMPP_ALWAYS_INLINE uint32_t get_previous(typename Set::const_iterator ls_id_iterator) const noexcept {
    if (ls_id_iterator != ls_id_set_.begin()) {
      return index_.index[*std::prev(ls_id_iterator)];
    }

    return 0;
  }

  PROMPP_ALWAYS_INLINE uint32_t get_next(typename Set::const_iterator ls_id_iterator) const noexcept {
    if (const auto next_iterator = std::next(ls_id_iterator); next_iterator != ls_id_set_.end()) {
      return index_.index[*next_iterator];
    }

    return kMaxIndexValue;
  }
};

}  // namespace series_index