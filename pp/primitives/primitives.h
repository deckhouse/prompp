#pragma once

#include <cstdint>
#include <limits>
#include <string>
#include <string_view>

#include "bare_bones/preprocess.h"

namespace PromPP::Primitives {

using Symbol = std::string;
using SymbolView = std::string_view;

// FIXME Label and LabelView should be convertable
using Label = std::pair<Symbol, Symbol>;
using LabelView = std::pair<SymbolView, SymbolView>;

using Timestamp = int64_t;
constexpr Timestamp kNullTimestamp = std::numeric_limits<int64_t>::min();

struct TimeInterval {
  static constexpr Timestamp kMin = std::numeric_limits<int64_t>::max();
  static constexpr Timestamp kMax = std::numeric_limits<int64_t>::min();

  Timestamp min{kMin};
  Timestamp max{kMax};

  PROMPP(inline) void reset(Timestamp min_value = kMin, Timestamp max_value = kMax) noexcept {
    min = min_value;
    max = max_value;
  }

  [[nodiscard]] PROMPP(inline) bool contains(Timestamp timestamp) const noexcept { return timestamp >= min && timestamp <= max; }
  [[nodiscard]] PROMPP(inline) bool intersect(const TimeInterval& interval) const noexcept {
    return std::max(min, interval.min) <= std::min(max, interval.max);
  }

  [[nodiscard]] PROMPP(inline) Timestamp difference() const noexcept { return max - min; }

  bool operator==(const TimeInterval&) const noexcept = default;
};

using LabelSetID = uint32_t;
constexpr LabelSetID kInvalidLabelSetID = std::numeric_limits<LabelSetID>::max();

}  // namespace PromPP::Primitives

namespace std {
PROMPP(inline) constexpr bool operator==(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return label_view.first == label.first && label_view.second == label.second;
}

PROMPP(inline) constexpr bool operator==(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return label_view == label;
}

PROMPP(inline) constexpr bool operator!=(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return !(label_view == label);
}

PROMPP(inline) constexpr bool operator!=(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return !(label_view == label);
}

PROMPP(inline) constexpr bool operator<(const PromPP::Primitives::Label& label, const PromPP::Primitives::LabelView& label_view) noexcept {
  return label.first < label_view.first && label.second < label_view.second;
}

PROMPP(inline) constexpr bool operator<(const PromPP::Primitives::LabelView& label_view, const PromPP::Primitives::Label& label) noexcept {
  return label_view.first < label.first && label_view.second < label.second;
}

PROMPP(inline) constexpr auto operator<=>(const PromPP::Primitives::LabelView& a, const PromPP::Primitives::LabelView& b) noexcept {
  if (const auto result = a.first <=> b.first; !std::is_eq(result)) {
    return result;
  }

  return a.second <=> b.second;
}

}  // namespace std
