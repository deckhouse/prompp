#pragma once

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "prometheus/label_matcher.h"

namespace series_index::querier {

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

using Cardinality = uint32_t;

template <class MatchType>
struct Matcher {
  BareBones::Vector<MatchType> matches{};
  MatchType label_name_id{};
  Cardinality cardinality{};
  PromPP::Prometheus::MatchStatus status{PromPP::Prometheus::MatchStatus::kUnknown};
  PromPP::Prometheus::MatcherType type{PromPP::Prometheus::MatcherType::kUnknown};

  auto operator<=>(const Matcher&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matches.empty(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_positive() const noexcept {
    using enum PromPP::Prometheus::MatcherType;
    return type == kExactMatch || type == kRegexpMatch;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_negative() const noexcept {
    using enum PromPP::Prometheus::MatcherType;
    return type == kExactNotMatch || type == kRegexpNotMatch;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unknown() const noexcept { return type == PromPP::Prometheus::MatcherType::kUnknown; }

  PROMPP_ALWAYS_INLINE void convert_to_negative() noexcept {
    using enum PromPP::Prometheus::MatcherType;

    if (type == kExactMatch) {
      type = kExactNotMatch;
    } else if (type == kRegexpMatch) {
      type = kRegexpNotMatch;
    }
  }

  PROMPP_ALWAYS_INLINE void convert_to_positive() noexcept {
    using enum PromPP::Prometheus::MatcherType;

    if (type == kExactNotMatch) {
      type = kExactMatch;
    } else if (type == kRegexpNotMatch) {
      type = kRegexpMatch;
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

}  // namespace series_index::querier

template <class MatchType>
struct BareBones::IsTriviallyReallocatable<series_index::querier::Matcher<MatchType>> : std::true_type {};

namespace series_index::querier {

template <class MatchType = MatchId>
struct Selector {
  using Matcher = series_index::querier::Matcher<MatchType>;
  using MatcherList = BareBones::Vector<Matcher>;
  using MatchList = BareBones::Vector<MatchType>;

  MatcherList matchers;

  bool operator<=>(const Selector&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return matchers.empty(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_positive_matchers() const noexcept {
    return std::ranges::any_of(matchers, [](const Matcher& matcher) PROMPP_LAMBDA_INLINE { return matcher.is_positive(); });
  }
};

}  // namespace series_index::querier