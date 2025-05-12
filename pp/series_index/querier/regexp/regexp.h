#pragma once

#include "re2/prog.h"
#include "re2/regexp.h"

#include "bare_bones/preprocess.h"

namespace series_index::querier::regexp {

class RegexpParser {
 public:
  using RegexpPtr = std::unique_ptr<re2::Regexp, void (*)(re2::Regexp*)>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static re2::Regexp::ParseFlags regexp_parse_flags() {
    return re2::Regexp::NeverCapture | re2::Regexp::MatchNL | re2::Regexp::PerlClasses | re2::Regexp::OneLine | re2::Regexp::PerlX;
  }

  [[nodiscard]] static RegexpPtr parse(std::string_view regexp) {
    re2::RegexpStatus parse_status;
    RegexpPtr rgx(re2::Regexp::Parse(regexp, regexp_parse_flags(), &parse_status), [](re2::Regexp* regexp) { regexp->Decref(); });
    if (!rgx) {
      return rgx;
    }

    if (const auto simplified_rgx = rgx->Simplify(); simplified_rgx != nullptr) {
      rgx.reset(simplified_rgx);
    } else {
      rgx = nullptr;
    }

    return rgx;
  }
};

class RegexpCompiledProg {
 public:
  RegexpCompiledProg() = default;
  explicit RegexpCompiledProg(re2::Regexp* rgx) { compile(rgx); }

  PROMPP_ALWAYS_INLINE bool compile(re2::Regexp* rgx) {
    prog_.reset(rgx->CompileToProg(0));
    return prog_ != nullptr;
  }

  [[nodiscard]] bool full_match(std::string_view str) const {
    // Drastically simplified logic from RE2::Match
    // https://github.com/google/re2/blob/2021-09-01/re2/re2.cc#L616

    if (!prog_) {
      return false;
    }

    bool dfa_failed;

    if (prog_->SearchDFA(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, &dfa_failed, nullptr)) {
      return true;
    }

    if (dfa_failed) {
      if (prog_->IsOnePass()) {
        return prog_->SearchOnePass(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
      }

      if (prog_->CanBitState() && str.size() <= static_cast<size_t>(256 * 1024 / prog_->list_count())) {
        return prog_->SearchBitState(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
      }

      return prog_->SearchNFA(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
    }

    return false;
  }

 private:
  std::unique_ptr<re2::Prog> prog_;
};

}  // namespace series_index::querier::regexp