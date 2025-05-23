#pragma once

#include <cstddef>

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

};  // namespace BareBones::iterator