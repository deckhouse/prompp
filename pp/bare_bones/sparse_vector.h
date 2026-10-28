#pragma once

#include "bitset.h"
#include "type_traits.h"
#include "vector.h"

namespace BareBones {

template <class T, template <class> class Container>
class SparseVector {
  Bitset element_exists_;
  Vector<uint32_t> element_positions_;
  Container<T> data_;

 public:
  [[nodiscard]] PROMPP(inline) bool empty() const noexcept { return !element_exists_.size(); }

  [[nodiscard]] PROMPP(inline) size_t size() const noexcept { return element_exists_.size(); }
  [[nodiscard]] PROMPP(inline) size_t items_count() const noexcept { return data_.size(); }

  PROMPP(inline) void resize(size_t size) noexcept {
    element_exists_.resize(size);
    element_positions_.resize(size);
  }

  PROMPP(inline) void clear() noexcept {
    element_exists_.clear();
    element_positions_.clear();
    data_.clear();
  }

  [[nodiscard]] PROMPP(inline) T& operator[](uint32_t key) noexcept {
    assert(key < element_exists_.size());

    if (element_exists_.is_set(key)) [[unlikely]] {
      return data_[element_positions_[key]];
    }

    element_exists_.set(key);
    element_positions_[key] = data_.size();
    return data_.emplace_back();
  }

  PROMPP(inline) const T& operator[](uint32_t key) const noexcept {
    assert(key < element_exists_.size());

    return data_[element_positions_[key]];
  }

  class IteratorSentinel {};

  class Iterator {
    const SparseVector* vec_{};
    Bitset::const_iterator i_{};

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = std::pair<uint32_t, const T&>;
    using difference_type = std::ptrdiff_t;

    PROMPP(inline) Iterator() noexcept = default;
    explicit PROMPP(inline) Iterator(const SparseVector* vec) noexcept : vec_(vec), i_(vec_->element_exists_.begin()) {}

    PROMPP(inline) auto operator*() const noexcept { return std::pair<uint32_t, const T&>(*i_, vec_->data_[vec_->element_positions_[*i_]]); }
    PROMPP(inline) Iterator& operator++() noexcept {
      ++i_;
      return *this;
    }
    PROMPP(inline) Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++i_;
      return retval;
    }
    PROMPP(inline) bool operator==(const Iterator& o) const noexcept { return i_ == o.i_; }
    PROMPP(inline) bool operator==(const IteratorSentinel&) const noexcept { return i_ == Bitset::IteratorSentinel(); }
  };

  using const_iterator = Iterator;

  PROMPP(inline) auto begin() const noexcept { return Iterator(this); }
  static PROMPP(inline) auto end() noexcept { return IteratorSentinel(); }
};

template <class T, template <class> class Container>
struct IsTriviallyReallocatable<SparseVector<T, Container>> : std::true_type {};

template <class T, template <class> class Container>
struct IsZeroInitializable<SparseVector<T, Container>> : std::true_type {};

}  // namespace BareBones
