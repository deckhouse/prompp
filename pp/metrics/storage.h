#pragma once

#include "metrics_page_list.h"

namespace metrics {

class Storage {
 public:
  class IteratorSentinel {};

  class Iterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = const Metric*;
    using difference_type = ptrdiff_t;
    using pointer = value_type;
    using reference = value_type;

    explicit Iterator(const MetricsPageList& storage) : page_iterator_(storage.begin()), metric_iterator_(*page_iterator_) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return metric_iterator_.operator*(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator->() const noexcept { return metric_iterator_.operator->(); }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      if (++metric_iterator_ == MetricsPageControlBlock::end()) [[likely]] {
        metric_iterator_ = MetricsPageControlBlock::Iterator(*++page_iterator_);
      }

      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return page_iterator_ == MetricsPageList::end(); }

   private:
    MetricsPageList::Iterator page_iterator_;
    MetricsPageControlBlock::Iterator metric_iterator_;
  };

  PROMPP_ALWAYS_INLINE void add(MetricsPageControlBlock* page) { page_list_.add(page); }
  PROMPP_ALWAYS_INLINE void remove_unused_pages() { page_list_.remove_unused_pages(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator(page_list_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE IteratorSentinel static end() noexcept { return {}; }

 private:
  MetricsPageList page_list_;
};

PROMPP_ALWAYS_INLINE Storage& storage() {
  static Storage storage;
  return storage;
}

template <class MetricsPageType, class... Args>
PROMPP_ALWAYS_INLINE MetricsPageType* CreateMetricsPage(Storage& s, Args&&... args) {
  auto* page = new MetricsPageType(std::forward<Args>(args)...);
  s.add(page);
  return page;
}

template <class MetricsPageType, class... Args>
PROMPP_ALWAYS_INLINE MetricsPageType* CreateMetricsPage(Args&&... args) {
  return CreateMetricsPage<MetricsPageType>(storage(), std::forward<Args>(args)...);
}

}  // namespace metrics