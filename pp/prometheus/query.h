#pragma once

#include <cstdint>

#include "label_matcher.h"
#include "primitives/primitives.h"

namespace PromPP::Prometheus {

template <class String, template <class> class Slice>
struct GenericSelectHints {
  Primitives::TimeInterval interval{.min = 0, .max = 0};
  int64_t limit{};

  int64_t step_ms{};
  String func{};

  Slice<String> grouping;
  int64_t range_ms{};

  uint64_t shard_count{};
  uint64_t shard_index{};

  int64_t lookback_delta_ms{};

  bool disable_trimming{};
  bool by{};

  GenericSelectHints() = default;

  template <class AnyString, template <class> class AnySlice>
    requires(!std::same_as<AnyString, String> || !std::same_as<AnySlice<AnyString>, Slice<String>>)
  explicit GenericSelectHints(const GenericSelectHints<AnyString, AnySlice>& hints)
      : interval(hints.interval),
        limit(hints.limit),
        step_ms(hints.step_ms),
        func(static_cast<std::string_view>(hints.func)),
        range_ms(hints.range_ms),
        shard_count(hints.shard_count),
        shard_index(hints.shard_index),
        lookback_delta_ms(hints.lookback_delta_ms),
        disable_trimming(hints.disable_trimming),
        by(hints.by) {
    if (!hints.grouping.empty()) [[unlikely]] {
      grouping.reserve(hints.grouping.size());
      for (const auto& group : hints.grouping) {
        grouping.emplace_back(static_cast<std::string_view>(group));
      }
    }
  }

  bool operator==(const GenericSelectHints&) const noexcept = default;
};

using SelectHints = GenericSelectHints<std::string, std::vector>;

struct Query {
  SelectHints hints{};
  LabelMatchers label_matchers;
  int64_t start_timestamp_ms{};
  int64_t end_timestamp_ms{};

  bool operator==(const Query&) const noexcept = default;
};

struct LabelValuesQuery {
  Query query{};
  std::string label_name;

  bool operator==(const LabelValuesQuery&) const noexcept = default;
};

}  // namespace PromPP::Prometheus
