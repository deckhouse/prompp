#pragma once

#include <algorithm>
#include <vector>

#include "bare_bones/preprocess.h"
#include "primitives/primitives.h"
#include "selector.h"
#include "series_index/reverse_index.h"

namespace series_index::querier {

using SeriesIdSpan = std::span<uint32_t>;

class MatchesMerger {
 public:
  SeriesIdSpan merge(const Selector<SeriesIdSequenceSnapshot>::Matcher::Matches& matches, uint32_t* memory) noexcept {
    merge_iterators_.clear();

    make_heap(matches);
    const auto end = merge(memory);

    return {memory, static_cast<size_t>(end - memory)};
  }

 private:
  [[no_unique_address]] struct IteratorGreaterByValue {
    PROMPP_ALWAYS_INLINE bool operator()(const SeriesIdSequenceSnapshot::Iterator& lhs, const SeriesIdSequenceSnapshot::Iterator& rhs) const noexcept {
      return *lhs > *rhs;
    }
  } greater_;

  std::vector<SeriesIdSequenceSnapshot::Iterator> merge_iterators_;

  PROMPP_ALWAYS_INLINE void make_heap(const Selector<SeriesIdSequenceSnapshot>::Matcher::Matches& matches) {
    merge_iterators_.reserve(matches.size());
    for (const auto& label_value_match : matches) {
      if (!label_value_match.empty()) {
        merge_iterators_.emplace_back(label_value_match.begin());
      }
    }

    std::ranges::make_heap(merge_iterators_, greater_);
  }

  PROMPP_ALWAYS_INLINE const uint32_t* merge(uint32_t* memory) {
    uint32_t previous_series_id = PromPP::Primitives::kInvalidLabelSetID;
    while (!merge_iterators_.empty()) {
      if (const auto value = *merge_iterators_.front(); value != previous_series_id) {
        *memory++ = value;
        previous_series_id = value;
      }

      if (auto& next = merge_iterators_.front(); ++next == SeriesIdSequenceSnapshot::end()) {
        std::ranges::pop_heap(merge_iterators_, greater_);
        merge_iterators_.pop_back();
      } else {
        sift_down_root();
      }
    }

    return memory;
  }

  PROMPP_ALWAYS_INLINE void sift_down_root() noexcept {
    const size_t size = merge_iterators_.size();
    for (size_t parent = 0;;) {
      size_t child = 2 * parent + 1;
      if (child >= size) {
        break;
      }

      if (child + 1 < size && greater_(merge_iterators_[child], merge_iterators_[child + 1])) {
        ++child;
      }

      if (!greater_(merge_iterators_[parent], merge_iterators_[child])) {
        break;
      }

      std::swap(merge_iterators_[parent], merge_iterators_[child]);
      parent = child;
    }
  }
};

class SetIntersecter {
 public:
  template <class Set2>
  PROMPP_ALWAYS_INLINE static SeriesIdSpan intersect(SeriesIdSpan set1, const Set2& set2) {
    return {set1.begin(), std::ranges::set_intersection(set1, set2, set1.begin()).out};
  }
};

class SetSubstractor {
 public:
  template <class Set2>
  PROMPP_ALWAYS_INLINE static SeriesIdSpan substract(SeriesIdSpan set1, const Set2& set2) {
    return {set1.begin(), std::ranges::set_difference(set1, set2, set1.begin()).out};
  }
};

}  // namespace series_index::querier