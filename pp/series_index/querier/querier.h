#pragma once

#include <ranges>

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
#include "selector.h"
#include "selector_querier.h"
#include "series_index/reverse_index.h"
#include "set_operations.h"

namespace series_index::querier {

template <class Index, template <class> class MemoryPoolContainer = std::vector>
class Querier {
 public:
  using Selector = querier::Selector<>;

  class MatchersComparatorByTypeAndCardinality {
   public:
    PROMPP_ALWAYS_INLINE bool operator()(const Selector::Matcher& a, const Selector::Matcher& b) const noexcept {
      if (a.is_positive()) {
        if (b.is_positive()) {
          return a.cardinality < b.cardinality;
        }

        return true;
      }

      return false;
    }
  };

  using SeriesIdContainer = MemoryPoolContainer<uint32_t>;

  struct QuerierResult {
    SeriesIdContainer series_ids{};
    QuerierStatus status{QuerierStatus::kNoMatch};

    PROMPP_ALWAYS_INLINE void set_series_id_list(SeriesIdContainer&& ids, uint32_t size) noexcept {
      series_ids = std::move(ids);
      series_ids.resize(size);
      status = series_ids.empty() ? QuerierStatus::kNoMatch : QuerierStatus::kMatch;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_error() const noexcept { return is_querier_status_error(status); }
  };

  explicit Querier(const Index& index) : index_(index) {}

  template <class LabelMatchers, MatchResolverInterface MatchResolver>
  [[nodiscard]] QuerierResult query(const LabelMatchers& label_matchers, const MatchResolver& match_resolver) {
    Selector selector;
    if (const auto status =
            SelectorQuerier<typename Index::TrieIndex, Selector, MatchResolver>{index_.trie_index(), match_resolver}.query(label_matchers, selector);
        status != QuerierStatus::kMatch) {
      return {.status = status};
    }

    return query(selector);
  }

  [[nodiscard]] QuerierResult query(Selector& selector) {
    QuerierResult result;
    MemoryPool memory_pool(fill_matchers_cardinality(selector));
    sort_matchers(selector);

    auto result_set = resolve_positive_matcher(selector.matchers[0], memory_pool.merge1, memory_pool.merge2);
    if (selector.matchers.size() > 1 && selector.matchers[1].is_positive()) {
      memory_pool.allocate_temp_memory();
    }

    for (auto it = std::next(selector.matchers.begin()); it != selector.matchers.end(); ++it) {
      process_matcher(*it, memory_pool, result_set);
    }

    result.set_series_id_list(memory_pool.release_container_for_merge(result_set.data()), result_set.size());
    return result;
  }

 private:
  class MemoryPool {
    SeriesIdContainer merge_container1_;
    SeriesIdContainer merge_container2_;
    SeriesIdContainer temp_container_;
    Cardinality cardinality_;

   public:
    uint32_t* merge1{};
    uint32_t* merge2{};
    uint32_t* temp{};

    explicit MemoryPool(uint32_t cardinality)
        : merge_container1_(cardinality),
          merge_container2_(cardinality),
          cardinality_(cardinality),
          merge1(merge_container1_.data()),
          merge2(merge_container2_.data()) {}

    PROMPP_ALWAYS_INLINE void allocate_temp_memory() {
      temp_container_.resize(cardinality_);
      temp = temp_container_.data();
    }

    PROMPP_ALWAYS_INLINE SeriesIdContainer&& release_container_for_merge(const uint32_t* memory) {
      if (memory == merge_container1_.data()) {
        return std::move(merge_container1_);
      }
      return std::move(merge_container2_);
    }
  };

  const Index& index_;
  SeriesSliceList series_slice_list_;

  PROMPP_ALWAYS_INLINE void sort_matchers(Selector& selector) const noexcept {
    std::sort(selector.matchers.begin(), selector.matchers.end(), MatchersComparatorByTypeAndCardinality{});
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Cardinality fill_matchers_cardinality(Selector& selector) const noexcept {
    Cardinality max_cardinality{};
    for (auto& matcher : selector.matchers) {
      if (need_resolve_matcher(matcher)) {
        matcher.cardinality = get_cardinality(matcher);
        max_cardinality = std::max(max_cardinality, matcher.cardinality);
      }
    }

    return max_cardinality;
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE bool need_resolve_matcher(const Selector::Matcher& matcher) noexcept {
    return matcher.is_positive() || (matcher.is_negative() && matcher.status == PromPP::Prometheus::MatchStatus::kAllMatchWithExcludes);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Cardinality get_cardinality(const Selector::Matcher& matcher) const noexcept {
    using enum PromPP::Prometheus::MatchStatus;

    if (BareBones::is_in(matcher.status, kAllMatch, kAllMatchWithExcludes)) {
      return index_.reverse_index().get(matcher.label_name_match)->count();
    }

    return BareBones::accumulate(matcher.matches, 0U, [this, &matcher](uint32_t cardinality, uint32_t label_value_id) PROMPP_LAMBDA_INLINE {
      return cardinality + index_.reverse_index().get(matcher.label_name_match, label_value_id)->count();
    });
  }

  PROMPP_ALWAYS_INLINE void process_matcher(const Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.is_positive()) {
      process_positive_matcher(matcher, memory_pool, result_set);
    } else if (matcher.is_negative()) {
      process_negative_matcher(matcher, memory_pool, result_set);
    }
  }

  PROMPP_ALWAYS_INLINE void process_positive_matcher(const Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      result_set = intersect_sequence(result_set, index_.reverse_index().get(matcher.label_name_match));
    } else {
      result_set = SetIntersecter::intersect(result_set, resolve_positive_matcher(matcher, memory_pool.merge2, memory_pool.temp));
    }
  }

  PROMPP_ALWAYS_INLINE void process_negative_matcher(const Selector::Matcher& matcher, MemoryPool& memory_pool, SeriesIdSpan& result_set) {
    if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatch) {
      result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_match));
    } else if (matcher.status == PromPP::Prometheus::MatchStatus::kPartialMatch) {
      result_set = substract_sequences(result_set, matcher);
    } else if (matcher.status == PromPP::Prometheus::MatchStatus::kAllMatchWithExcludes) {
      result_set = SetSubstractor::substract(result_set, resolve_all_match_with_excludes_matcher(matcher, memory_pool.merge2));
    }
  }

  SeriesIdSpan resolve_positive_matcher(const Selector::Matcher& matcher, uint32_t*& memory, uint32_t*& temp_memory) {
    using enum PromPP::Prometheus::MatchStatus;

    if (matcher.status == kAllMatch) {
      return resolve_all_match_matcher(matcher, memory);
    }

    if (matcher.status == kAllMatchWithExcludes) {
      return resolve_all_match_with_excludes_matcher(matcher, memory);
    }

    return resolve_partial_match_matcher(matcher, memory, temp_memory);
  }

  PROMPP_ALWAYS_INLINE SeriesIdSpan resolve_all_match_matcher(const Selector::Matcher& matcher, uint32_t* memory) {
    auto sequence = index_.reverse_index().get(matcher.label_name_match);
    decode_sequence(sequence, memory);
    return {memory, sequence->count()};
  }

  PROMPP_ALWAYS_INLINE SeriesIdSpan resolve_all_match_with_excludes_matcher(const Selector::Matcher& matcher, uint32_t* memory) {
    auto sequence = index_.reverse_index().get(matcher.label_name_match);
    decode_sequence(sequence, memory);
    return substract_sequences(SeriesIdSpan{memory, sequence->count()}, matcher);
  }

  PROMPP_ALWAYS_INLINE SeriesIdSpan resolve_partial_match_matcher(const Selector::Matcher& matcher, uint32_t*& memory, uint32_t*& temp_memory) {
    series_slice_list_.clear();
    series_slice_list_.reserve(matcher.matches.size());

    uint32_t offset = 0;
    for (auto label_value_id : matcher.matches) {
      auto sequence = index_.reverse_index().get(matcher.label_name_match, label_value_id);
      decode_sequence(sequence, memory + offset);
      series_slice_list_.emplace_back(SeriesSlice{.begin = offset, .end = offset + sequence->count()});
      offset += sequence->count();
    }

    return SetMerger::merge(series_slice_list_, memory, temp_memory);
  }

  PROMPP_ALWAYS_INLINE static void decode_sequence(const SeriesIdSequence* sequence, uint32_t* memory) { std::ranges::copy(*sequence, memory); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SeriesIdSpan intersect_sequence(SeriesIdSpan result_set, const SeriesIdSequence* sequence) {
    return SetIntersecter::intersect(result_set, *sequence);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SeriesIdSpan substract_sequence(SeriesIdSpan result_set, const SeriesIdSequence* sequence) {
    return SetSubstractor::substract(result_set, *sequence);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesIdSpan substract_sequences(SeriesIdSpan result_set, const Selector::Matcher& matcher) {
    for (auto label_value_id : matcher.matches) {
      result_set = substract_sequence(result_set, index_.reverse_index().get(matcher.label_name_match, label_value_id));
    }

    return result_set;
  }
};

}  // namespace series_index::querier
