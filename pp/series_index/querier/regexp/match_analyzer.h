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
    if (!regexp) {
      return Status::kError;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpEmptyMatch) {
      return Status::kEmptyMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpStar && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
      return Status::kAnythingMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpPlus && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
      return Status::kAllMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpConcat) {
      if (const auto i = skip_begin_text_anchor(regexp); i == skip_end_text_anchor(regexp, i)) {
        if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpPlus) {
          if (regexp->sub()[i]->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
            return Status::kAllMatch;
          }
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpStar) {
          if (regexp->sub()[i]->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
            return Status::kAnythingMatch;
          }
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpEndText) {
          return Status::kEmptyMatch;
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpAlternate) {
          if (has_empty_alternative(regexp->sub()[i])) {
            return Status::kAllMatchWithExcludes;
          }
        }
      }
    } else if (regexp->op() == re2::RegexpOp::kRegexpAlternate) {
      if (has_empty_alternative(regexp)) {
        return Status::kAllMatchWithExcludes;
      }
    }

    return Status::kPartialMatch;
  }

  [[nodiscard]] static int skip_begin_text_anchor(re2::Regexp* regexp) noexcept {
    int i = 0;
    while (i < regexp->nsub() && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpBeginText) {
      ++i;
    }

    return i;
  }

  [[nodiscard]] static int skip_end_text_anchor(re2::Regexp* regexp, int start) noexcept {
    int i = regexp->nsub() - 1;
    while (i > start && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpEndText) {
      --i;
    }
    return i;
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool has_empty_alternative(re2::Regexp* regexp) noexcept {
    using enum re2::RegexpOp;

    for (auto i = 0; i < regexp->nsub(); ++i) {
      if (const auto alternative = regexp->sub()[i]; alternative->nsub() == 0) {
        if (BareBones::is_in(alternative->op(), kRegexpEmptyMatch, kRegexpBeginText, kRegexpEndText)) {
          return true;
        }
      } else {
        if (const auto start = skip_begin_text_anchor(alternative); start == alternative->nsub() || alternative->sub()[start]->op() == kRegexpEndText) {
          return true;
        }
      }
    }

    return false;
  }
};

}  // namespace series_index::querier::regexp