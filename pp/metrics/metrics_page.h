#pragma once

#include "metric.h"

namespace metrics {

class MetricsPageControlBlock {
 public:
  virtual ~MetricsPageControlBlock() = default;

  class IteratorSentinel {};

  class Iterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = const Metric*;
    using difference_type = ptrdiff_t;
    using pointer = value_type;
    using reference = value_type;

    explicit Iterator(const MetricsPageControlBlock* metrics_page) : metrics_page_(metrics_page), offset_(metrics_page_ ? metrics_page_->metric_offset() : 0) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return metric(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator->() const noexcept { return metric(); }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      offset_ += metric()->object_size();
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept {
      return metrics_page_ == nullptr || offset_ == metrics_page_->page_object_size();
    }

   private:
    const MetricsPageControlBlock* metrics_page_;
    uint32_t offset_;

    [[nodiscard]] const Metric* metric() const { return reinterpret_cast<const Metric*>(reinterpret_cast<const uint8_t*>(metrics_page_) + offset_); }
  };

  explicit MetricsPageControlBlock(uint32_t page_object_size) : page_object_size_(page_object_size) {}
  MetricsPageControlBlock(uint32_t page_object_size, uint32_t metric_offset) : page_object_size_(page_object_size), metric_offset_(metric_offset) {}

  MetricsPageControlBlock() = delete;
  MetricsPageControlBlock(const MetricsPageControlBlock&) = delete;
  MetricsPageControlBlock(MetricsPageControlBlock&&) noexcept = delete;

  MetricsPageControlBlock& operator=(const MetricsPageControlBlock&) = delete;
  MetricsPageControlBlock& operator=(MetricsPageControlBlock&&) noexcept = delete;

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsPageControlBlock* next_metrics_page() const noexcept { return next_metrics_page_; }
  PROMPP_ALWAYS_INLINE void set_next_metrics_page(MetricsPageControlBlock* next_metrics_page) noexcept { next_metrics_page_ = next_metrics_page; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t page_object_size() const noexcept { return page_object_size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t metric_offset() const noexcept { return metric_offset_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unused() const noexcept { return ref_count_ == 0; }
  PROMPP_ALWAYS_INLINE void detach() noexcept { ref_count_ = 0; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return Iterator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE IteratorSentinel static end() noexcept { return {}; }

 private:
  MetricsPageControlBlock* next_metrics_page_{};
  uint32_t ref_count_{1};
  const uint32_t page_object_size_;
  const uint32_t metric_offset_{sizeof(MetricsPageControlBlock)};
};

template <class Derived>
class MetricsPage : public MetricsPageControlBlock {
 public:
  static_assert(!std::is_pointer_v<Derived>);

  using MetricsPageControlBlock::MetricsPageControlBlock;

  explicit MetricsPage() : MetricsPageControlBlock(sizeof(Derived)) {
    static_assert(sizeof(Derived) >= sizeof(MetricsPageControlBlock) + sizeof(Metric), "Metrics page must contain at least one metric");
  }
  explicit MetricsPage(const Metric& first_metric)
      : MetricsPageControlBlock(sizeof(Derived), reinterpret_cast<const char*>(&first_metric) - reinterpret_cast<const char*>(this)) {}

  MetricsPage(const MetricsPage&) = delete;
  MetricsPage(MetricsPage&&) noexcept = delete;

  MetricsPage& operator=(const MetricsPage&) = delete;
  MetricsPage& operator=(MetricsPage&&) noexcept = delete;
};

}  // namespace metrics