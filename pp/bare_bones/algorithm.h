#pragma once

#include <utility>

namespace BareBones {

// implementation for std::ranges::accumulate
// TODO: remove this function on C++23 and use std::ranges::fold_left instead
template <class Range, class ValueType, class Method>
ValueType accumulate(const Range& range, ValueType initial_value, Method&& method) {
  for (const auto& item : range) {
    initial_value = method(initial_value, item);
  }

  return initial_value;
}

template <class Value, class... Args>
constexpr bool is_in(const Value& value, Args&&... args) {
  return (... || (value == std::forward<Args>(args)));
}

template <class ResultType, class... Args>
constexpr ResultType build_bitmask(Args&&... args) {
  return (... | static_cast<ResultType>(std::forward<Args>(args)));
}

template <class Range1, class Range2, class Comparator>
auto lexicographical_compare_three_way(const Range1& range1, const Range2& range2, Comparator&& comparator) {
  using result_type = decltype(comparator(*range1.begin(), *range2.begin()));

  auto it_a = range1.begin();
  auto it_b = range2.begin();
  for (; it_a != range1.end() && it_b != range2.end(); ++it_a, ++it_b) {
    if (const auto result = comparator(*it_a, *it_b); !std::is_eq(result)) {
      return result;
    }
  }

  if (it_a == range1.end()) {
    if (it_b == range2.end()) {
      return result_type::equal;
    }

    return result_type::less;
  }

  return result_type::greater;
}

};  // namespace BareBones
