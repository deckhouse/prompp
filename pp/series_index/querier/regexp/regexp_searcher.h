#pragma once

#include <ranges>

#include "match_analyzer.h"
#include "prometheus/label_matcher.h"
#include "series_index/trie/concepts.h"

namespace series_index::querier::regexp {

template <class Trie, trie::RegexpMatchesListInterface<typename Trie::Traversal> MatchesList>
class RegexpSearcher {
 public:
  explicit RegexpSearcher(MatchesList& matches) : matches_(matches) {}

  [[nodiscard]] PromPP::Prometheus::MatchStatus search(const Trie& trie, const RegexpPtr& regexp) {
    auto matches_count_before = matches_.count();
    process_subtrie(kProcessSubTrieDepthLimit, trie.make_traversal(), regexp.get());
    if (matches_.count() == matches_count_before) {
      return PromPP::Prometheus::MatchStatus::kEmptyMatch;
    }

    return PromPP::Prometheus::MatchStatus::kPartialMatch;
  }

 private:
  static constexpr uint8_t kProcessSubTrieDepthLimit = 50;

  MatchesList& matches_;
  re2::Regexp* prepared_for_ = nullptr;
  RegexpCompiledProg prepared_prog_;

  void process_subtrie(uint8_t depth_limit, const typename Trie::Traversal& trv, re2::Regexp* rgx) {
    if (depth_limit == 0) {
      process_subtrie_by_regexp(trv, rgx);
      return;
    }

    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpAlternate: {
        for (int i = 0; i < rgx->nsub(); i++) {
          process_subtrie(depth_limit - 1, trv, rgx->sub()[i]);
        }
        break;
      }

      case re2::RegexpOp::kRegexpConcat: {
        process_concatenated_regexp(depth_limit, trv, rgx);
        break;
      }

      case re2::RegexpOp::kRegexpLiteral:
      case re2::RegexpOp::kRegexpLiteralString:
      case re2::RegexpOp::kRegexpCharClass: {
        process_exact_prefix(depth_limit, trv, rgx);
        break;
      }

      case re2::RegexpOp::kRegexpEmptyMatch: {
        process_one_exact_prefix(depth_limit, trv, "");
        break;
      }

      default: {
        process_subtrie_by_regexp(trv, rgx);
      }
    }
  }

  void process_concatenated_regexp(uint8_t depth_limit, const typename Trie::Traversal& trv, re2::Regexp* rgx) {
    const auto i = RegexpMatchAnalyzer::skip_begin_text_anchor(rgx);
    if (i >= rgx->nsub()) [[unlikely]] {
      process_one_exact_prefix(depth_limit, trv, "");
      return;
    }

    using enum re2::RegexpOp;
    if (BareBones::is_in(rgx->sub()[i]->op(), kRegexpLiteral, kRegexpLiteralString, kRegexpCharClass)) {
      const std::span sub_regexps{rgx->sub() + i + 1, rgx->sub() + rgx->nsub()};

      if (sub_regexps.size() > 1) {
        const auto rgx_tail = concatenate(sub_regexps.data(), static_cast<int>(sub_regexps.size()), rgx->parse_flags());
        process_exact_prefix(depth_limit, trv, rgx->sub()[i], rgx_tail.get());
      } else if (!sub_regexps.empty()) {
        process_exact_prefix(depth_limit, trv, rgx->sub()[i], sub_regexps.front());
      } else {
        process_exact_prefix(depth_limit, trv, rgx->sub()[i]);
      }

      return;
    }

    process_subtrie_by_regexp(trv, rgx);
  }

  void process_exact_prefix(uint8_t depth_limit, const typename Trie::Traversal& trv, re2::Regexp* rgx, re2::Regexp* rgx_tail = nullptr) {
    char buf[re2::UTFmax + 1];

    // Do simple full scan if it's a case-insensitive regex
    if (rgx->parse_flags() & re2::Regexp::FoldCase) {
      if (rgx_tail) {
        std::array rgxs{rgx, rgx_tail};
        const auto concat_rgx = concatenate(rgxs.data(), rgxs.size(), rgx->parse_flags());
        process_subtrie_by_regexp(trv, concat_rgx.get());
      } else {
        process_subtrie_by_regexp(trv, rgx);
      }
      return;
    }

    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpLiteral: {
        const auto& r = rgx->rune();
        process_one_exact_prefix(depth_limit, trv, std::string_view(buf, re2::runetochar(buf, &r)), rgx_tail);
        break;
      }

      case re2::RegexpOp::kRegexpLiteralString: {
        std::string literal;
        if (rgx->parse_flags() & re2::Regexp::Latin1) {
          literal.resize(rgx->nrunes());
          for (int i = 0; i < rgx->nrunes(); i++) {
            literal[i] = static_cast<char>(rgx->runes()[i]);
          }
        } else {
          literal.resize(static_cast<size_t>(rgx->nrunes()) * re2::UTFmax);
          char* p = &literal[0];
          for (int i = 0; i < rgx->nrunes(); i++)
            p += re2::runetochar(p, rgx->runes() + i);
          literal.resize(p - &literal[0]);
        }
        process_one_exact_prefix(depth_limit, trv, literal, rgx_tail);
        break;
      }

      case re2::RegexpOp::kRegexpCharClass: {
        if (rgx->cc()->size() < 100) {
          for (const auto& rr : *rgx->cc()) {
            for (auto r = rr.lo; r <= rr.hi; ++r) {
              process_one_exact_prefix(depth_limit, trv, std::string_view(buf, re2::runetochar(buf, &r)), rgx_tail);
            }
          }
        } else {
          if (!rgx_tail) {
            process_subtrie_by_regexp(trv, rgx);
          } else {
            std::array rgxs{rgx, rgx_tail};
            const auto concat_rgx = concatenate(rgxs.data(), rgxs.size(), rgx->parse_flags());
            process_subtrie_by_regexp(trv, concat_rgx.get());
          }
        }
        break;
      }

      default: {
        // can't get here
        assert(false);
      }
    }
  }

  void process_one_exact_prefix(uint8_t depth_limit, const typename Trie::Traversal& trv, const std::string_view& prefix, re2::Regexp* rgx_tail = nullptr) {
    if (!rgx_tail) {
      matches_.add_leaf(trv, prefix);
      return;
    }

    auto ntrv = trv;
    if (!ntrv.traverse(prefix)) {
      return;
    }

    if (!ntrv.is_leaf()) {
      process_subtrie(depth_limit - 1, ntrv, rgx_tail);
      return;
    }

    auto tail = ntrv.tail();
    switch (rgx_tail->op()) {
      case re2::RegexpOp::kRegexpEmptyMatch: {
        if (tail.empty()) {
          matches_.add_leaf(ntrv);
        }
        break;
      }

      case re2::RegexpOp::kRegexpPlus:
      case re2::RegexpOp::kRegexpStar: {
        if (rgx_tail->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          if (rgx_tail->op() == re2::RegexpOp::kRegexpStar || tail.size() > 0) {
            matches_.add_leaf(ntrv);
          }
          break;
        }

        [[fallthrough]];
      }

      default: {
        if (prepare_regexp(rgx_tail)) {
          if (prepared_prog_.full_match(tail)) {
            matches_.add_leaf(ntrv);
          }
        }
      }
    }
  }

  void process_subtrie_by_regexp(const typename Trie::Traversal& trv, re2::Regexp* rgx) {
    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpPlus: {
        if (rgx->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          matches_.add_subnodes(trv);
          return;
        }

        break;
      }

      case re2::RegexpOp::kRegexpStar: {
        if (rgx->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          matches_.add_node(trv);
          return;
        }

        break;
      }

      case re2::RegexpOp::kRegexpConcat: {
        if (rgx->sub()[rgx->nsub() - 1]->op() == re2::RegexpOp::kRegexpEndText) {
          const auto j = RegexpMatchAnalyzer::skip_end_text_anchor(rgx, 0);
          const auto unanchored_rgx = concatenate(rgx->sub(), j + 1, rgx->parse_flags());
          process_subtrie_by_regexp(trv, unanchored_rgx.get());
          return;
        }

        break;
      }

      default: {
        break;
      };
    }

    if (prepare_regexp(rgx)) {
      matches_.add_node(trv, [this](std::string_view node_tail) PROMPP_LAMBDA_INLINE { return prepared_prog_.full_match(node_tail); });
    }
  }

  PROMPP_ALWAYS_INLINE bool prepare_regexp(re2::Regexp* rgx) {
    if (prepared_for_ == rgx) {
      return true;
    }

    if (!prepared_prog_.compile(rgx)) {
      return false;
    }

    prepared_for_ = rgx;
    return true;
  }
};

}  // namespace series_index::querier::regexp