#pragma once

#include <iterator>
#include <ranges>

#include "allocated_memory.h"
#include "preprocess.h"
#include "vector.h"

#include "bitset.h"

namespace BareBones {

namespace VectorWithHolesImpl {

template <class T>
concept holes_need_bitset = concepts::has_allocated_memory<T> || concepts::dereferenceable_has_allocated_memory<T> || !IsTriviallyDestructible<T>::value;

template <typename T, typename... Args>
concept destroyable_with = requires(T t, Args... args) {
  { t.destroy(std::forward<Args>(args)...) };
};

#pragma pack(push, 1)
template <class T>
union ItemOrHole {
  template <class... Args>
  explicit PROMPP(inline) ItemOrHole(Args&&... args) : value{std::forward<Args>(args)...} {}

  PROMPP(inline) ~ItemOrHole() {}

  template <class... Args>
  PROMPP(inline) void create_item(Args&&... args) {
    std::construct_at(&value, std::forward<Args>(args)...);
  }
  template <class... Args>
  PROMPP(inline) void destroy_item(Args&&... args) {
    if constexpr (destroyable_with<T, Args...>) {
      value.destroy(std::forward<Args>(args)...);
    }
    std::destroy_at(&value);
  }

  [[nodiscard]] PROMPP(inline) size_t allocated_memory() const noexcept { return mem::allocated_memory(value); }

  T value;
  uint32_t next_hole;
};
#pragma pack(pop)
}  // namespace VectorWithHolesImpl

template <class T>
struct IsTriviallyReallocatable<VectorWithHolesImpl::ItemOrHole<T>> : std::true_type {};

template <class T, ReallocatorInterface Reallocator = DefaultReallocator>
class VectorWithHoles {
 public:
  ~VectorWithHoles() {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      for_each_item([&](auto& item) { item.destroy_item(); });
    }
  }

  template <class... Args>
  PROMPP(inline) T& emplace_back(Args&&... args) noexcept {
    if (has_holes()) [[unlikely]] {
      auto& item = vector_[next_hole_];

      unmark_hole(next_hole_);
      next_hole_ = vector_[next_hole_].next_hole;

      item.create_item(std::forward<Args>(args)...);
      return item.value;
    }
    return vector_.emplace_back(std::forward<Args>(args)...).value;
  }

  template <class... Args>
  PROMPP(inline) void erase(uint32_t index, Args&&... args) {
    assert(index < vector_.size());
    assert(!is_hole(index));

    vector_[index].destroy_item(std::forward<Args>(args)...);

    vector_[index].next_hole = next_hole_;
    next_hole_ = index;

    mark_hole(index);
  }

  PROMPP(inline) uint32_t index_of(const T& item) const noexcept { return reinterpret_cast<const Item*>(&item) - vector_.begin(); }

  PROMPP(inline) const T& operator[](uint32_t i) const noexcept { return vector_[i].value; }
  PROMPP(inline) T& operator[](uint32_t i) noexcept { return vector_[i].value; }

  PROMPP(inline) const T& at(uint32_t i) const {
    if (is_hole(i)) {
      throw Exception(0x8749d003112fc0c4, "VectorWithHoles::at: attempt to access a hole");
    }
    return vector_[i].value;
  }
  PROMPP(inline) T& at(uint32_t i) {
    if (is_hole(i)) {
      throw Exception(0xa4ce63143b8d187c, "VectorWithHoles::at: attempt to access a hole");
    }
    return vector_[i].value;
  }

  [[nodiscard]] PROMPP(inline) uint32_t size() const noexcept { return vector_.size(); }

  // Skips holes; without the holes bitset finding them would cost a walk of the hole list per item.
  class ConstIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = T;
    using difference_type = std::ptrdiff_t;
    using pointer = const T*;
    using reference = const T&;

    ConstIterator() = default;
    PROMPP(inline) ConstIterator(const VectorWithHoles& vector, uint32_t index) noexcept : vector_(&vector), index_(index) { skip_holes(); }

    [[nodiscard]] PROMPP(inline) reference operator*() const noexcept { return (*vector_)[index_]; }
    [[nodiscard]] PROMPP(inline) pointer operator->() const noexcept { return &(*vector_)[index_]; }

    PROMPP(inline) ConstIterator& operator++() noexcept {
      ++index_;
      skip_holes();
      return *this;
    }
    PROMPP(inline) ConstIterator operator++(int) noexcept {
      const ConstIterator retval = *this;
      ++*this;
      return retval;
    }

    [[nodiscard]] PROMPP(inline) bool operator==(const ConstIterator& other) const noexcept { return index_ == other.index_; }

   private:
    const VectorWithHoles* vector_{};
    uint32_t index_{};

    PROMPP(inline) void skip_holes() noexcept {
      while (index_ < vector_->size() && vector_->is_hole(index_)) {
        ++index_;
      }
    }
  };

  [[nodiscard]] PROMPP(inline) ConstIterator begin() const noexcept
    requires VectorWithHolesImpl::holes_need_bitset<T>
  {
    return ConstIterator(*this, 0);
  }
  [[nodiscard]] PROMPP(inline) ConstIterator end() const noexcept
    requires VectorWithHolesImpl::holes_need_bitset<T>
  {
    return ConstIterator(*this, size());
  }

  [[nodiscard]] PROMPP(inline) size_t allocated_memory() const noexcept {
    size_t allocated_memory = vector_.capacity() * sizeof(Item);
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      allocated_memory += holes_index_set_.allocated_memory();
      for_each_item([&](const auto& item) { allocated_memory += mem::allocated_memory(item); });
    }

    return allocated_memory;
  }

 private:
  using Item = VectorWithHolesImpl::ItemOrHole<T>;
  struct nothing_t {};
  // The bitset was chosen as the main data structure based on series_data_encoder_benchmark
  // between Roaring::roaring and BareBones::Bitset
  using BitsetOrEmpty = std::conditional_t<VectorWithHolesImpl::holes_need_bitset<T>, GenericBitset<Reallocator>, nothing_t>;

  Vector<Item, Reallocator> vector_;
  [[no_unique_address]] BitsetOrEmpty holes_index_set_;
  uint32_t next_hole_{std::numeric_limits<uint32_t>::max()};

  [[nodiscard]] PROMPP(inline) bool has_holes() const noexcept { return next_hole_ != std::numeric_limits<uint32_t>::max(); }
  [[nodiscard]] PROMPP(inline) bool is_hole(uint32_t index) const noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      return holes_index_set_.is_set(index);
    } else {
      if (index >= vector_.size()) {
        return false;
      }
      for (uint32_t current = next_hole_; current != std::numeric_limits<uint32_t>::max(); current = vector_[current].next_hole) {
        if (current == index) {
          return true;
        }
      }
      return false;
    }
  }

  PROMPP(inline) void mark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      holes_index_set_.set(index);
    }
  }
  PROMPP(inline) void unmark_hole(uint32_t index) noexcept {
    if constexpr (VectorWithHolesImpl::holes_need_bitset<T>) {
      holes_index_set_.reset(index);
    }
  }

  // skips holes and applies f for each item
  template <class UnaryFunc>
  PROMPP(inline) void for_each_item(UnaryFunc&& f) {
    for_each_item_impl(std::forward<UnaryFunc>(f), *this);
  }

  // const variant
  template <class UnaryFunc>
  PROMPP(inline) void for_each_item(UnaryFunc&& f) const {
    for_each_item_impl(std::forward<UnaryFunc>(f), *this);
  }

  template <class UnaryFunc, class Self>
  static PROMPP(inline) void for_each_item_impl(UnaryFunc&& f, Self&& self) {
    size_t index_global = 0;
    const auto& holes_index_set = self.holes_index_set_;
    auto& vector = self.vector_;

    for (auto hole_it = holes_index_set.begin(); hole_it != holes_index_set.end(); ++hole_it) {
      for (size_t index = index_global, index_end = *hole_it; index < index_end; ++index) {
        f(vector[index]);
      }
      index_global = (*hole_it) + 1;
    }

    for (size_t index = index_global; index < vector.size(); ++index) {
      f(vector[index]);
    }
  }
};

}  // namespace BareBones

// VectorWithHoles::size() counts holes, while iteration skips them; disable sized-range fast paths.
namespace std::ranges {

template <class T, BareBones::ReallocatorInterface Reallocator>
inline constexpr bool disable_sized_range<BareBones::VectorWithHoles<T, Reallocator>> = true;

}  // namespace std::ranges
