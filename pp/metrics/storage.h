#pragma once

#include <atomic>

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

    explicit Iterator(const MetricsPageList& storage, uint32_t generation)
        : page_iterator_(storage.begin()), metric_iterator_(*page_iterator_), generation_(generation) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t generation() const noexcept { return generation_; }

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
    uint32_t generation_;
  };

  PROMPP_ALWAYS_INLINE void add(MetricsPageControlBlock* page) { page_list_.add(page); }
  PROMPP_ALWAYS_INLINE void remove_unused_pages(uint32_t generation) { page_list_.remove_unused_pages(generation); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t current_generation() const noexcept { return generation_.load(std::memory_order_relaxed); }

  // begin() opens a new scrape: it bumps the storage generation and stamps the iterator with it. Detached pages recorded in
  // earlier generations become eligible for deletion via remove_unused_pages(iterator.generation()) after the scrape.
  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() noexcept { return Iterator(page_list_, generation_.fetch_add(1, std::memory_order_relaxed) + 1); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE IteratorSentinel static end() noexcept { return {}; }

 private:
  std::atomic<uint32_t> generation_{0};
  MetricsPageList page_list_;
};

inline Storage storage;

template <class MetricsPageType, class... Args>
MetricsPageType* CreateMetricsPage(Storage& s, Args&&... args) {
  auto* page = new MetricsPageType(std::forward<Args>(args)...);
  s.add(page);
  return page;
}

template <class MetricsPageType, class... Args>
MetricsPageType* CreateMetricsPage(Args&&... args) {
  return CreateMetricsPage<MetricsPageType>(storage, std::forward<Args>(args)...);
}

}  // namespace metrics