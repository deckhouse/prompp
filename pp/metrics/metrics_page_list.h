#pragma once

#include "metrics_page.h"

namespace metrics {

class MetricsPageList {
 public:
  class IteratorSentinel {};

  class Iterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = MetricsPageControlBlock*;
    using difference_type = ptrdiff_t;
    using pointer = value_type;
    using reference = value_type;

    explicit Iterator(MetricsPageControlBlock* metrics_page) : metrics_page_(metrics_page) { advance_to_non_empty_metrics_page(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return metrics_page_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator->() const noexcept { return metrics_page_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      metrics_page_ = metrics_page_->next_metrics_page();
      advance_to_non_empty_metrics_page();
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return metrics_page_ == nullptr; }

   private:
    MetricsPageControlBlock* metrics_page_;

    void advance_to_non_empty_metrics_page() noexcept {
      while (metrics_page_ != nullptr && metrics_page_->is_unused()) {
        metrics_page_ = metrics_page_->next_metrics_page();
      }
    }
  };

  ~MetricsPageList() {
    MetricsPageControlBlock* page = next_metrics_page_;
    while (page) {
      delete std::exchange(page, page->next_metrics_page());
    }
  }

  void add(MetricsPageControlBlock* page) {
    MetricsPageControlBlock* current_next_page = next_metrics_page_;

    do {
      page->set_next_metrics_page(current_next_page);
    } while (!next_metrics_page_.compare_exchange_weak(current_next_page, page));
  }

  void remove_unused_pages() {
    MetricsPageControlBlock* page = next_metrics_page_;
    if (page == nullptr) [[unlikely]] {
      return;
    }

    if (MetricsPageControlBlock* next_page = page->next_metrics_page(); next_page != nullptr) [[likely]] {
      remove_unused_pages(page, next_page);
    }

    if (page->is_unused()) {
      // If page is first page in list then we delete it. Otherwise, we will delete it at another RemoveUnusedPages call
      if (next_metrics_page_.compare_exchange_weak(page, page->next_metrics_page())) [[likely]] {
        delete page;
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator(next_metrics_page_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE IteratorSentinel static end() noexcept { return {}; }

 private:
  std::atomic<MetricsPageControlBlock*> next_metrics_page_{};

  static void remove_unused_pages(MetricsPageControlBlock* prev_page, MetricsPageControlBlock* page) {
    do {
      const auto next_page = page->next_metrics_page();

      if (page->is_unused()) {
        prev_page->set_next_metrics_page(next_page);
        delete page;
      } else {
        prev_page = page;
      }

      page = next_page;
    } while (page != nullptr);
  }
};

}  // namespace metrics