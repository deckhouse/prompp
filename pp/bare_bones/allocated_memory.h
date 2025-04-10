#pragma once

#include "concepts.h"
#include "preprocess.h"

namespace BareBones::mem {
namespace concepts {
using BareBones::concepts::dereferenceable_has_allocated_memory;
using BareBones::concepts::has_allocated_memory;
}  // namespace concepts

template <class T>
[[nodiscard]] constexpr PROMPP_ALWAYS_INLINE size_t allocated_memory(T&& value) {
  if constexpr (mem::concepts::has_allocated_memory<T>) {
    return value.allocated_memory();
  } else if constexpr (mem::concepts::dereferenceable_has_allocated_memory<T>) {
    return value->allocated_memory();
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
