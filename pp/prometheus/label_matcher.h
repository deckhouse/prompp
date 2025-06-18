#pragma once

#include <algorithm>
#include <limits>
#include <string>
#include <vector>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus {

enum class MatcherType : uint8_t {
  kExactMatch = 0,
  kExactNotMatch,
  kRegexpMatch,
  kRegexpNotMatch,
  kUnknown,
};

[[nodiscard]] PROMPP_ALWAYS_INLINE MatcherType MatcherTypeFromInt(int32_t type) noexcept {
  if (type < static_cast<int32_t>(MatcherType::kExactMatch) || type >= static_cast<int32_t>(MatcherType::kUnknown)) {
    return MatcherType::kUnknown;
  }

  return static_cast<MatcherType>(type);
}

template <class StringType>
struct LabelMatcherTrait {
  StringType name{};
  StringType value{};
  MatcherType type{MatcherType::kUnknown};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return !name.empty() && !value.empty() && type != MatcherType::kUnknown; }

  PROMPP_ALWAYS_INLINE void invalidate() noexcept { type = MatcherType::kUnknown; }

  PROMPP_ALWAYS_INLINE void set_default_protobuf_values() noexcept { type = MatcherType::kExactMatch; }

  auto operator<=>(const LabelMatcherTrait&) const noexcept = default;
};

using LabelMatcher = LabelMatcherTrait<std::string>;
using LabelMatchers = std::vector<LabelMatcher>;

enum class MatchStatus : uint8_t {
  kUnknown = 0,
  kEmptyMatch,
  kAllMatch,
  kAllMatchWithExcludes,
  kPartialMatch,
  kError,
};

class MatchId {
 public:
  MatchId() = default;
  MatchId(const MatchId&) noexcept = default;
  MatchId(MatchId&&) noexcept = default;

  // NOLINTNEXTLINE(google-explicit-constructor)
  MatchId(uint32_t id) : id_{id} {}

  MatchId& operator=(const MatchId&) noexcept = default;
  MatchId& operator=(uint32_t id) noexcept {
    id_ = id;
    return *this;
  }

  auto operator<=>(const MatchId&) const noexcept = default;
  auto operator<=>(uint32_t id) const noexcept { return id_ <=> id; }

  // NOLINTNEXTLINE(google-explicit-constructor)
  operator uint32_t() const noexcept { return id_; }

 private:
  uint32_t id_{std::numeric_limits<uint32_t>::max()};
};

template <class MatchType = MatchId>
struct Selector {
  struct Matcher {
    using Cardinality = uint32_t;

    std::vector<MatchType> matches{};
    MatchType label_name_id{};
    Cardinality cardinality{};
    MatchStatus status{MatchStatus::kUnknown};
    MatcherType type{MatcherType::kUnknown};

    auto operator<=>(const Matcher&) const noexcept = default;

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matches.empty(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_positive() const noexcept { return type == MatcherType::kExactMatch || type == MatcherType::kRegexpMatch; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_negative() const noexcept { return type == MatcherType::kExactNotMatch || type == MatcherType::kRegexpNotMatch; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unknown() const noexcept { return type == MatcherType::kUnknown; }

    PROMPP_ALWAYS_INLINE void convert_to_negative() noexcept {
      if (type == MatcherType::kExactMatch) {
        type = MatcherType::kExactNotMatch;
      } else if (type == MatcherType::kRegexpMatch) {
        type = MatcherType::kRegexpNotMatch;
      }
    }

    PROMPP_ALWAYS_INLINE void convert_to_positive() noexcept {
      if (type == MatcherType::kExactNotMatch) {
        type = MatcherType::kExactMatch;
      } else if (type == MatcherType::kRegexpNotMatch) {
        type = MatcherType::kRegexpMatch;
      }
    }

    PROMPP_ALWAYS_INLINE void invert() noexcept {
      if (is_positive()) {
        convert_to_negative();
      } else if (is_negative()) {
        convert_to_positive();
      }
    }
  };

  std::vector<Matcher> matchers;

  bool operator<=>(const Selector&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matchers.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_positive_matchers() const noexcept {
    return std::ranges::any_of(matchers, [](const Matcher& matcher) PROMPP_LAMBDA_INLINE { return matcher.is_positive(); });
  }
};

}  // namespace PromPP::Prometheus