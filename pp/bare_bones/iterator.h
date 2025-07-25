#pragma once

#include <algorithm>
#include <cassert>
#include <cstddef>
#include <cstdint>

#include "preprocess.h"

namespace BareBones::iterator {

template <class Operation>
class OperationIterator {
 public:
  using difference_type = ptrdiff_t;

  explicit OperationIterator(Operation& operation) : operation_(&operation) {}

  Operation& operator*() const noexcept { return *operation_; }
  OperationIterator& operator++() noexcept { return *this; }
  OperationIterator operator++(int) const noexcept { return *this; }

 private:
  Operation* operation_;
};

template <class Iterator, class IteratorSentinel>
class BatchIterator {
 public:
  using difference_type = typename Iterator::difference_type;
  using iterator_category = std::input_iterator_tag;
  using value_type = typename Iterator::value_type;
  using pointer = value_type*;
  using reference = value_type&;

  PROMPP_ALWAYS_INLINE BatchIterator(Iterator&& iterator, uint32_t batch_size) noexcept : iterator_(std::move(iterator)), batch_size_(batch_size) {
    assert(batch_size > 0);
  }

  [[nodiscard]] uint32_t batch_size() const noexcept { return batch_size_; }
  void next_batch() { processed_ = 0; }

  PROMPP_ALWAYS_INLINE BatchIterator& operator++() noexcept {
    ++processed_;
    ++iterator_;
    return *this;
  }

  PROMPP_ALWAYS_INLINE BatchIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  auto operator*() const noexcept { return *iterator_; }
  auto operator*() noexcept { return *iterator_; }
  auto operator->() const noexcept { return iterator_.operator->(); }
  auto operator->() noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel& end) const noexcept { return processed_ == batch_size_ || iterator_ == end; }

 private:
  Iterator iterator_;
  uint32_t batch_size_;
  uint32_t processed_{};
};

};  // namespace BareBones::iterator