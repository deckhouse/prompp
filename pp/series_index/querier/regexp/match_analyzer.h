#pragma once

#include "bare_bones/algorithm.h"
#include "regexp.h"

namespace series_index::querier::regexp {

class RegexpMatchAnalyzer {
 public:
  enum class Status : uint8_t {
    kError = 0,
    kEmptyMatch,
    kAnythingMatch,
    kAllMatch,
    kAllMatchWithExcludes,
    kPartialMatch,
  };

  [[nodiscard]] static Status analyze(re2::Regexp* regexp) {
    if (!regexp) [[unlikely]] {
      return Status::kError;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpConcat) {
      if (const auto significant_sub_regexp = get_significant_sub_regexp(regexp); significant_sub_regexp) [[likely]] {
        return analyze_sub_regexp(significant_sub_regexp);
      }

      return Status::kPartialMatch;
    }

    return analyze_sub_regexp(regexp);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int skip_begin_text_anchor(re2::Regexp* regexp) noexcept {
    int i = 0;
    while (i < regexp->nsub() && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpBeginText) {
      ++i;
    }

    return i;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int skip_end_text_anchor(re2::Regexp* regexp, int start) noexcept {
    int i = regexp->nsub() - 1;
    while (i > start && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpEndText) {
      --i;
    }
    return i;
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static re2::Regexp* get_significant_sub_regexp(re2::Regexp* regexp) noexcept {
    if (const auto i = skip_begin_text_anchor(regexp); i == skip_end_text_anchor(regexp, i)) {
      // NOLINTNEXTLINE(clang-analyzer-security.ArrayBound)
      return regexp->sub()[i];
    }

    return nullptr;
  }

  [[nodiscard]] static Status analyze_sub_regexp(re2::Regexp* regexp) noexcept {
    if (BareBones::is_in(regexp->op(), re2::RegexpOp::kRegexpEmptyMatch, re2::RegexpOp::kRegexpEndText)) {
      return Status::kEmptyMatch;
    }

    if (is_anything_match(regexp)) {
      return Status::kAnythingMatch;
    }

    if (is_all_match(regexp)) {
      return Status::kAllMatch;
    }

    if (has_empty_alternative(regexp)) {
      return Status::kAllMatchWithExcludes;
    }

    return Status::kPartialMatch;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool is_anything_match(re2::Regexp* regexp) noexcept {
    return regexp->op() == re2::RegexpOp::kRegexpStar && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool is_all_match(re2::Regexp* regexp) noexcept {
    return regexp->op() == re2::RegexpOp::kRegexpPlus && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar;
  }

  [[nodiscard]] static bool has_empty_alternative(re2::Regexp* regexp) noexcept {
    using enum re2::RegexpOp;

    if (regexp->op() != kRegexpAlternate) {
      return false;
    }

    for (auto i = 0; i < regexp->nsub(); ++i) {
      if (const auto alternative = regexp->sub()[i]; alternative->nsub() == 0) {
        if (BareBones::is_in(alternative->op(), kRegexpEmptyMatch, kRegexpBeginText, kRegexpEndText)) {
          return true;
        }
      } else {
        // NOLINTNEXTLINE(clang-analyzer-security.ArrayBound)
        if (const auto start = skip_begin_text_anchor(alternative); start == alternative->nsub() || alternative->sub()[start]->op() == kRegexpEndText) {
          return true;
        }
      }
    }

    return false;
  }
};

}  // namespace series_index::querier::regexp