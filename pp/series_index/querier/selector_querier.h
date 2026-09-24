#pragma once

#include <cassert>

#include "match_resolver.h"
#include "prometheus/label_matcher.h"
#include "regexp/regexp_searcher.h"

namespace series_index::querier {

enum class QuerierStatus : uint8_t {
  kNoPositiveMatchers = 0,
  kRegexpError,
  kNoMatch,
  kMatch,
};

PROMPP_ALWAYS_INLINE bool is_querier_status_error(QuerierStatus status) noexcept {
  return status != QuerierStatus::kMatch && status != QuerierStatus::kNoMatch;
}

template <class TrieIndex, class Selector, MatchResolverInterface MatchResolver>
class SelectorQuerier {
 public:
  using MatcherType = PromPP::Prometheus::MatcherType;
  using MatchStatus = PromPP::Prometheus::MatchStatus;
  using Trie = TrieIndex::Trie;

  static constexpr auto kInvalidLabelNameId = std::numeric_limits<uint32_t>::max();

  explicit SelectorQuerier(const TrieIndex& index, const MatchResolver& match_resolver) : index_(index), match_resolver_(match_resolver) {}

  template <class LabelMatchers>
  [[nodiscard]] PROMPP_ALWAYS_INLINE QuerierStatus query(const LabelMatchers& label_matchers, Selector& selector) {
    const auto status = query_impl(label_matchers, selector);
    excluded_value_ids_ = {};
    return status;
  }

 private:
  const TrieIndex& index_;
  const MatchResolver& match_resolver_;
  BareBones::Vector<uint32_t> excluded_value_ids_;

  template <class LabelMatchers>
  [[nodiscard]] QuerierStatus query_impl(const LabelMatchers& label_matchers, Selector& selector) {
    selector.matchers.reserve(label_matchers.size());

    for (auto& label_matcher : label_matchers) {
      auto& matcher = selector.matchers.emplace_back();
      matcher.type = label_matcher.type;

      if (matcher.is_positive()) {
        if (auto status = query(label_matcher, matcher, selector.matchers); status != QuerierStatus::kMatch) {
          return status;
        }
      }
    }

    const auto matchers_size = label_matchers.size();
    for (size_t i = 0; i < matchers_size; ++i) {
      auto& label_matcher = label_matchers[i];
      auto& matcher = selector.matchers[i];

      if (matcher.is_negative() && matcher.status == MatchStatus::kUnknown) {
        if (auto status = query(label_matcher, matcher, selector.matchers); status != QuerierStatus::kMatch && status != QuerierStatus::kNoMatch) {
          return status;
        }

        if (selector.matchers[i].is_unknown()) {
          return QuerierStatus::kNoMatch;
        }
      }
    }

    if (!selector.have_positive_matchers()) [[unlikely]] {
      return QuerierStatus::kNoPositiveMatchers;
    }

    return QuerierStatus::kMatch;
  }

  template <class LabelMatcher>
  PROMPP_ALWAYS_INLINE QuerierStatus query(const LabelMatcher& label_matcher, Selector::Matcher& matcher, Selector::MatcherList& matchers) {
    return query_values(label_matcher, get_values_trie(label_matcher, matcher), matcher, matchers);
  }

  template <class LabelMatcher>
  PROMPP_ALWAYS_INLINE uint32_t get_values_trie(const LabelMatcher& label_matcher, Selector::Matcher& matcher) const noexcept {
    if (auto index = index_.names_trie().lookup(static_cast<std::string_view>(label_matcher.name)); index) {
      const auto name_id = *index;
      matcher.label_name_match = match_resolver_.resolve_name(name_id);
      return name_id;
    }

    return kInvalidLabelNameId;
  }

  template <class LabelMatcher>
  QuerierStatus query_values(const LabelMatcher& label_matcher, uint32_t label_name_id, Selector::Matcher& matcher, Selector::MatcherList& matchers) {
    if (label_matcher.value.empty()) {
      process_empty_matcher(matcher, label_name_id);
      return QuerierStatus::kMatch;
    }

    switch (matcher.type) {
      case MatcherType::kExactMatch:
      case MatcherType::kExactNotMatch: {
        return query_exact_value(label_matcher, label_name_id, matcher);
      }

      case MatcherType::kRegexpMatch:
      case MatcherType::kRegexpNotMatch: {
        return query_values_by_regexp(label_matcher, label_name_id, matcher, matchers);
      }

      default: {
        assert(false);
        return QuerierStatus::kNoMatch;
      }
    }
  }

  template <class LabelMatcher>
  QuerierStatus query_exact_value(const LabelMatcher& label_matcher, uint32_t label_name_id, Selector::Matcher& matcher) {
    if (label_name_id == kInvalidLabelNameId) {
      matcher.status = MatchStatus::kEmptyMatch;
      return QuerierStatus::kNoMatch;
    }

    if (auto value = index_.existing_values_trie(label_name_id).lookup(static_cast<std::string_view>(label_matcher.value)); value) {
      matcher.matches.emplace_back(match_resolver_.value_resolver(label_name_id)(*value));
      matcher.status = MatchStatus::kPartialMatch;
      return QuerierStatus::kMatch;
    }

    matcher.status = MatchStatus::kEmptyMatch;
    return QuerierStatus::kNoMatch;
  }

  static void process_empty_matcher(Selector::Matcher& matcher, uint32_t label_name_id) {
    if (matcher.is_positive()) {
      matcher.convert_to_negative();

      if (label_name_id != kInvalidLabelNameId) {
        matcher.status = MatchStatus::kAllMatch;
      } else {
        matcher.status = MatchStatus::kEmptyMatch;
      }
    } else {
      if (label_name_id != kInvalidLabelNameId) {
        matcher.convert_to_positive();

        matcher.status = MatchStatus::kAllMatch;
      } else {
        matcher.status = MatchStatus::kEmptyMatch;
        matcher.type = MatcherType::kUnknown;
      }
    }
  }

  template <class LabelMatcher>
  QuerierStatus query_values_by_regexp(const LabelMatcher& label_matcher, uint32_t label_name_id, Selector::Matcher& matcher, Selector::MatcherList& matchers) {
    const auto regexp = regexp::RegexpParser::parse(static_cast<std::string_view>(label_matcher.value));
    switch (regexp::RegexpMatchAnalyzer::analyze(regexp.get())) {
      using enum regexp::RegexpMatchAnalyzer::Status;

      case kError: {
        matcher.status = MatchStatus::kError;
        return QuerierStatus::kRegexpError;
      }

      case kAllMatch: {
        if (label_name_id == kInvalidLabelNameId) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.status = MatchStatus::kAllMatch;
        return QuerierStatus::kMatch;
      }

      case kAllMatchWithExcludes: {
        if (label_name_id == kInvalidLabelNameId) {
          matcher.status = MatchStatus::kEmptyMatch;
          matcher.type = MatcherType::kUnknown;
          return QuerierStatus::kMatch;
        }

        if (matcher.is_positive()) {
          matcher.status =
              regexp_search_with_excludes(regexp, label_name_id, matcher) == MatchStatus::kEmptyMatch ? MatchStatus::kAllMatch : MatchStatus::kPartialMatch;
        } else {
          matcher.status = regexp_search(regexp, label_name_id, matcher) == MatchStatus::kEmptyMatch ? MatchStatus::kAllMatch : MatchStatus::kPartialMatch;
        }

        if (matcher.status == MatchStatus::kPartialMatch) {
          if (matcher.is_positive()) {
            matcher.invert();
          } else {
            matchers.emplace_back(typename Selector::Matcher{
                .label_name_match = matcher.label_name_match,
                .status = MatchStatus::kAllMatch,
                .type = MatcherType::kRegexpMatch,
            });
          }
        } else {
          matcher.invert();
        }

        return QuerierStatus::kMatch;
      }

      case kPartialMatch: {
        if (label_name_id == kInvalidLabelNameId) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        if (const auto status = regexp_search(regexp, label_name_id, matcher); status == MatchStatus::kEmptyMatch) {
          matcher.status = MatchStatus::kEmptyMatch;
          return QuerierStatus::kNoMatch;
        }

        matcher.status = MatchStatus::kPartialMatch;
        return QuerierStatus::kMatch;
      }

      case kEmptyMatch: {
        process_empty_matcher(matcher, label_name_id);
        return QuerierStatus::kMatch;
      }

      case kAnythingMatch: {
        matcher.status = MatchStatus::kAllMatch;
        matcher.type = MatcherType::kUnknown;
        return QuerierStatus::kMatch;
      }

      default: {
        assert(false);
        return QuerierStatus::kRegexpError;
      }
    }
  }

  PROMPP_ALWAYS_INLINE PromPP::Prometheus::MatchStatus regexp_search(const regexp::RegexpPtr& regexp,
                                                                     uint32_t label_name_id,
                                                                     Selector::Matcher& matcher) noexcept {
    const auto value_resolver = match_resolver_.value_resolver(label_name_id);
    typename TrieIndex::Trie::MatchesList matches_list(matcher.matches, value_resolver);
    return regexp::RegexpSearcher<typename TrieIndex::Trie, decltype(matches_list)>(matches_list).search(*index_.values_trie(label_name_id), regexp);
  }

  PromPP::Prometheus::MatchStatus regexp_search_with_excludes(const regexp::RegexpPtr& regexp, uint32_t label_name_id, Selector::Matcher& matcher) noexcept {
    const auto& values_trie = *index_.values_trie(label_name_id);
    const auto status = regexp_search_excludes(regexp, values_trie);
    if (status == MatchStatus::kEmptyMatch) {
      return status;
    }

    fill_matcher_without_excludes(values_trie.count(), match_resolver_.value_resolver(label_name_id), matcher.matches);
    return status;
  }

  PROMPP_ALWAYS_INLINE PromPP::Prometheus::MatchStatus regexp_search_excludes(const regexp::RegexpPtr& regexp, const auto& values_trie) noexcept {
    excluded_value_ids_.clear();

    ValueMatchIdResolver value_id_resolver;
    typename TrieIndex::Trie::MatchesList matches_list(excluded_value_ids_, value_id_resolver);
    return regexp::RegexpSearcher<typename TrieIndex::Trie, decltype(matches_list)>(matches_list).search(values_trie, regexp);
  }

  void fill_matcher_without_excludes(uint32_t values_trie_count, const auto& value_resolver, Selector::Matcher::Matches& matches) {
    std::ranges::sort(excluded_value_ids_);

    matches.reserve(values_trie_count);

    uint32_t value_id = 0;
    for (const auto excluded_value_id : excluded_value_ids_) {
      for (; value_id < excluded_value_id; ++value_id) {
        matches.emplace_back(value_resolver(value_id));
      }

      value_id = excluded_value_id + 1;
    }

    for (; value_id < values_trie_count; ++value_id) {
      matches.emplace_back(value_resolver(value_id));
    }

    if (matches.empty()) [[unlikely]] {
      matches.shrink_to_fit();
    }
  }
};

}  // namespace series_index::querier