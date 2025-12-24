#pragma once

#include "bare_bones/xxhash.h"

namespace PromPP::Primitives::hash {

template <class LabelSet>
size_t hash_of_label_set(const LabelSet& label_set) noexcept {
  BareBones::XXHash hash;
  for (const auto& [label_name, label_value] : label_set) {
    hash.extend(static_cast<std::string_view>(label_name), static_cast<std::string_view>(label_value));
  }
  return static_cast<size_t>(hash);
}

template <class Iterator, class IteratorSentinel>
size_t hash_of_label_set(Iterator begin, IteratorSentinel end) noexcept {
  BareBones::XXHash hash;
  for (; begin != end; ++begin) {
    const auto& [label_name, label_value] = *begin;
    hash.extend(static_cast<std::string_view>(label_name), static_cast<std::string_view>(label_value));
  }

  return static_cast<size_t>(hash);
}

template <class LabelSet>
size_t hash_of_label_set(const LabelSet& label_set, bool drop_metric_name) noexcept {
  BareBones::XXHash hash;
  for (const auto& [label_name, label_value] : label_set) {
    if (drop_metric_name && label_name == "__name__") [[unlikely]] {
      continue;
    }

    hash.extend(static_cast<std::string_view>(label_name), static_cast<std::string_view>(label_value));
  }
  return static_cast<size_t>(hash);
}

template <class StringList>
size_t hash_of_string_list(const StringList& strings) noexcept {
  BareBones::XXHash hash;
  for (const auto& string : strings) {
    hash.extend(static_cast<std::string_view>(string));
  }
  return static_cast<size_t>(hash);
}

}  // namespace PromPP::Primitives::hash
