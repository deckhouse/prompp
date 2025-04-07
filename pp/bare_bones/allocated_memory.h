#pragma once

#include "algorithm.h"
#include "concepts.h"
#include "preprocess.h"

namespace BareBones::mem {
namespace concepts {
using BareBones::concepts::dereferenceable_has_allocated_memory;
using BareBones::concepts::has_allocated_memory;
}  // namespace concepts

template <class Container>
concept is_container_with_memory_allocation = requires(const Container container) {
  typename Container::value_type;

  { container.capacity() } -> std::convertible_to<size_t>;
};

template <class T>
[[nodiscard]] constexpr PROMPP_ALWAYS_INLINE size_t allocated_memory(T&& value) {
  if constexpr (mem::concepts::has_allocated_memory<T>) {
    return value.allocated_memory();
  } else if constexpr (mem::concepts::dereferenceable_has_allocated_memory<T>) {
    return value->allocated_memory();
  } else if constexpr (is_container_with_memory_allocation<std::remove_reference_t<T>>) {
    return value.capacity() * sizeof(typename std::remove_reference_t<T>::value_type) +
           BareBones::accumulate(value, 0ULL, [](size_t size, const auto& item) { return size + allocated_memory(item); });
  } else {
    return 0;
  }
}

template <>
[[nodiscard]] constexpr PROMPP_ALWAYS_INLINE size_t allocated_memory(const std::string& value) {
  return value.capacity() > std::string{}.capacity() ? value.capacity() : 0;
}

template <>
[[nodiscard]] constexpr PROMPP_ALWAYS_INLINE size_t allocated_memory(const std::pair<std::string, std::string>& value) {
  return allocated_memory(value.first) + allocated_memory(value.second);
}

}  // namespace BareBones::mem
