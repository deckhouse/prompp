#pragma once

#include <atomic>
#include <cstring>
#include <limits>

#include "allocator.h"
#include "preprocess.h"
#include "type_traits.h"

namespace BareBones {

template <class DataType, class SizeType>
class PreAllocationSizeCalculator {
 public:
  static constexpr size_t kMinAllocationSize = 32;

  [[nodiscard]] static PROMPP(inline) uint32_t calculate(SizeType needed_size) noexcept {
    if (needed_size > std::numeric_limits<uint32_t>::max()) [[unlikely]] {
      std::abort();
    }

    if constexpr (sizeof(DataType) < 8) {
      if (static_cast<double>(needed_size) * 1.5 * sizeof(SizeType) < 256) {
        // grow 50%, round up to 32b
        return ((static_cast<size_t>(needed_size * sizeof(DataType) * 1.5) & 0xFFFFFFFFFFFFFFE0) + kMinAllocationSize) / sizeof(DataType);
      }
    }

    if (static_cast<double>(needed_size) * 1.5 * sizeof(DataType) < 4096) {
      // grow 50%, round up to 256b
      return ((static_cast<size_t>(needed_size * sizeof(DataType) * 1.5) & 0xFFFFFFFFFFFFFF00) + 256) / sizeof(DataType);
    }

    // grow 10%, round up to 4096b
    const auto new_size = ((static_cast<size_t>(needed_size * sizeof(DataType) * 1.1) & 0xFFFFFFFFFFFFF000) + 4096) / sizeof(DataType);
    return std::min(new_size, static_cast<size_t>(std::numeric_limits<uint32_t>::max()));
  }
};

template <class Derived, class SizeType, class T, ReallocatorInterface Reallocator = DefaultReallocator>
class GenericMemory {
 public:
  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  PROMPP(inline) void resize_to_fit_at_least(SizeType needed_size) noexcept { derived()->resize(get_allocation_size(needed_size)); }

  PROMPP(inline) void grow_to_fit_at_least(SizeType needed_size) noexcept {
    if (needed_size > size()) {
      resize_to_fit_at_least(needed_size);
    }
  }

  PROMPP(inline) void grow_to_fit_at_least_and_fill_with_zeros(SizeType needed_size) noexcept {
    if (needed_size > size()) {
      const auto old_size = size();
      resize_to_fit_at_least(needed_size);
      std::memset(begin() + old_size, 0, (size() - old_size) * sizeof(T));
    }
  }

  [[nodiscard]] PROMPP(inline) bool empty() const noexcept { return size() == 0; }
  [[nodiscard]] PROMPP(inline) SizeType size() const noexcept { return derived()->get_size(); }

  [[nodiscard]] PROMPP(inline) const T* begin() const noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP(inline) T* begin() noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP(inline) const T* end() const noexcept { return derived()->data() + size(); }
  [[nodiscard]] PROMPP(inline) T* end() noexcept { return derived()->data() + size(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP(inline) operator const T*() const noexcept { return begin(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP(inline) operator T*() noexcept { return begin(); }

 private:
  [[nodiscard]] static PROMPP(inline) SizeType get_allocation_size(SizeType needed_size) noexcept {
    if constexpr (PROMPP(test) || PROMPP(asan)) {
      // In unit tests or in build with asan we allocate only needed_size bytes. It helps us to debug memory access errors
      return needed_size;
    } else {
      return Reallocator::allocation_size(PreAllocationSizeCalculator<T, SizeType>::calculate(needed_size) * sizeof(T)) / sizeof(T);
    }
  }

  PROMPP(inline) Derived* derived() noexcept { return static_cast<Derived*>(this); }
  PROMPP(inline) const Derived* derived() const noexcept { return static_cast<const Derived*>(this); }
};

template <template <class> class ControlBlock, class T>
concept MemoryControlBlockInterface = requires(ControlBlock<T> control_block) {
  typename ControlBlock<T>::SizeType;

  { control_block.data } -> std::same_as<T*&>;
  { control_block.data_size } -> std::same_as<typename ControlBlock<T>::SizeType&>;
};

template <class T>
struct MemoryControlBlock {
  using SizeType = uint32_t;

  T* data{};
  SizeType data_size{};
};

template <class T>
struct MemoryControlBlockWithItemCount {
  using SizeType = uint32_t;

  T* data{};
  SizeType data_size{};
  SizeType items_count{};
};

template <template <class> class ControlBlock, class T, ReallocatorInterface Reallocator = DefaultReallocator>
  requires MemoryControlBlockInterface<ControlBlock, T>
class Memory : public GenericMemory<Memory<ControlBlock, T, Reallocator>, typename ControlBlock<T>::SizeType, T, Reallocator> {
 public:
  using SizeType = typename ControlBlock<T>::SizeType;

  PROMPP(inline) Memory() noexcept = default;
  PROMPP(inline) Memory(const Memory& o) noexcept { copy(o); }
  PROMPP(inline) Memory(Memory&& o) noexcept : control_block_(std::exchange(o.control_block_, {})) {};
  PROMPP(inline) ~Memory() noexcept { Reallocator::free(control_block_.data); }

  PROMPP(inline) Memory& operator=(const Memory& o) noexcept {
    if (this != &o) [[likely]] {
      copy(o);
    }

    return *this;
  }

  PROMPP(inline) Memory& operator=(Memory&& o) noexcept {
    if (this != &o) [[likely]] {
      Reallocator::free(control_block_.data);
      control_block_ = std::exchange(o.control_block_, {});
    }

    return *this;
  }

  [[nodiscard]] PROMPP(inline) auto& control_block() noexcept { return control_block_; }
  [[nodiscard]] PROMPP(inline) const auto& control_block() const noexcept { return control_block_; }
  [[nodiscard]] PROMPP(inline) size_t allocated_memory() const noexcept { return control_block_.data_size * sizeof(T); }

 protected:
  friend class GenericMemory<Memory, SizeType, T, Reallocator>;

  [[nodiscard]] PROMPP(inline) SizeType get_size() const noexcept { return control_block_.data_size; }
  [[nodiscard]] PROMPP(inline) T* data() noexcept { return control_block_.data; }
  [[nodiscard]] PROMPP(inline) const T* data() const noexcept { return control_block_.data; }

  PROMPP(inline) void resize(SizeType new_size) noexcept {
    PROMPP(diagnostic, push)
    PROMPP(diagnostic, ignore, class_memaccess)
    control_block_.data = static_cast<T*>(Reallocator::reallocate(control_block_.data, new_size * sizeof(T)));
    PROMPP(diagnostic, pop)

    if (control_block_.data == nullptr && new_size > 0) [[unlikely]] {
      std::abort();
    }

    control_block_.data_size = new_size;
  }

 private:
  ControlBlock<T> control_block_;

  PROMPP(inline) void copy(const Memory& o) noexcept {
    static_assert(IsTriviallyCopyable<T>::value, "it's not allowed to copy memory for non trivially copyable types");

    T* data = control_block_.data;
    control_block_ = o.control_block_;
    control_block_.data = data;

    resize(control_block_.data_size);

    PROMPP(diagnostic, push)
    PROMPP(diagnostic, ignore, class_memaccess)
    // NOLINTNEXTLINE(clang-diagnostic-nontrivial-memcall)
    std::memcpy(control_block_.data, o.control_block_.data, control_block_.data_size * sizeof(T));
    PROMPP(diagnostic, pop)
  }
};

template <class ControlBlock>
concept SharedPtrControlBlockInterface = requires(ControlBlock control_block, const ControlBlock const_control_block) {
  requires std::integral<typename ControlBlock::RefCounter>;
  requires std::integral<typename ControlBlock::ItemCounter>;

  { control_block.ref_count() } -> std::same_as<typename ControlBlock::RefCounter&>;
  { const_control_block.ref_count() } -> std::same_as<typename ControlBlock::RefCounter>;
  { control_block.atomic_ref_count() } -> std::same_as<typename ControlBlock::AtomicRefCounter>;

  { control_block.items_count() } -> std::same_as<typename ControlBlock::ItemCounter>;
  { control_block.set_items_count(typename ControlBlock::ItemCounter()) };
};

class SharedPtrControlBlockWithItemCount {
 public:
  using RefCounter = uint32_t;
  using ItemCounter = uint32_t;
  using AtomicRefCounter = std::atomic_ref<RefCounter>;

  [[nodiscard]] PROMPP(inline) ItemCounter items_count() const noexcept { return items_count_; }
  PROMPP(inline) void set_items_count(ItemCounter count) noexcept { items_count_ = count; }

  [[nodiscard]] PROMPP(inline) RefCounter& ref_count() noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) RefCounter ref_count() const noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) AtomicRefCounter atomic_ref_count() noexcept { return AtomicRefCounter(ref_count_); }

 private:
  RefCounter ref_count_{1};
  ItemCounter items_count_{};
};

static_assert(SharedPtrControlBlockInterface<SharedPtrControlBlockWithItemCount>);

class AtomicSharedPtrControlBlockWithItemCount {
 public:
  using RefCounter = uint32_t;
  using ItemCounter = uint32_t;
  using AtomicRefCounter = std::atomic_ref<RefCounter>;

  [[nodiscard]] PROMPP(inline) ItemCounter items_count() const noexcept { return items_count_.load(std::memory_order_acquire); }
  PROMPP(inline) void set_items_count(ItemCounter count) noexcept { items_count_.store(count, std::memory_order_release); }

  [[nodiscard]] PROMPP(inline) RefCounter& ref_count() noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) RefCounter ref_count() const noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) AtomicRefCounter atomic_ref_count() noexcept { return AtomicRefCounter(ref_count_); }

 private:
  RefCounter ref_count_{1};
  std::atomic<ItemCounter> items_count_{};
};

static_assert(SharedPtrControlBlockInterface<AtomicSharedPtrControlBlockWithItemCount>);

class SharedPtrControlBlock {
 public:
  using RefCounter = uint32_t;
  using ItemCounter = uint32_t;
  using AtomicRefCounter = std::atomic_ref<RefCounter>;

  [[nodiscard]] static PROMPP(inline) ItemCounter items_count() noexcept { return 0; }
  static PROMPP(inline) void set_items_count(ItemCounter) noexcept {}

  [[nodiscard]] PROMPP(inline) RefCounter& ref_count() noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) RefCounter ref_count() const noexcept { return ref_count_; }
  [[nodiscard]] PROMPP(inline) AtomicRefCounter atomic_ref_count() noexcept { return AtomicRefCounter(ref_count_); }

 private:
  RefCounter ref_count_{1};
};

static_assert(SharedPtrControlBlockInterface<SharedPtrControlBlock>);

template <class T, SharedPtrControlBlockInterface ControlBlockType, ReallocatorInterface Reallocator>
class SharedPtr {
 public:
  using ControlBlock = ControlBlockType;

  static_assert(IsTriviallyReallocatable<T>::value);
  static_assert(IsTriviallyDestructible<T>::value);

  static constexpr uint32_t kControlBlockSize = sizeof(ControlBlock);

  SharedPtr() = default;
  PROMPP(inline) SharedPtr(uint32_t size, ControlBlock::ItemCounter items_count) : data_(nullptr) {
    non_atomic_reallocate(size);
    set_items_count(items_count);
  }
  PROMPP(inline) SharedPtr(const SharedPtr& other) noexcept : data_(other.data_) { inc_ref_counter(); }
  PROMPP(inline) SharedPtr(SharedPtr&& other) noexcept : data_(std::exchange(other.data_, nullptr)) {}

  PROMPP(inline) ~SharedPtr() { dec_ref_counter(); }

  PROMPP(inline) SharedPtr& operator=(const SharedPtr& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = other.data_;
      inc_ref_counter();
    }

    return *this;
  }

  PROMPP(inline) SharedPtr& operator=(SharedPtr&& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = std::exchange(other.data_, nullptr);
    }

    return *this;
  }

  PROMPP(inline) void reallocate(uint32_t old_size, uint32_t new_size) {
    if (non_atomic_is_unique()) [[likely]] {
      non_atomic_reallocate(new_size);
    } else {
      const SharedPtr old(std::move(*this));

      // NOLINTNEXTLINE(clang-analyzer-cplusplus.Move)
      non_atomic_reallocate(new_size);
      set_items_count(old.items_count());
      if (old_size > 0) [[likely]] {
        PROMPP(diagnostic, push)
        PROMPP(diagnostic, ignore, class_memaccess)
        PROMPP(diagnostic, ignore, stringop_overread)
        PROMPP(diagnostic, ignore, array_bounds)
        std::memcpy(data_, old.get(), std::min(old_size, new_size) * sizeof(T));
        PROMPP(diagnostic, pop)
      }
    }
  }

  [[nodiscard]] PROMPP(inline) ControlBlock::RefCounter non_atomic_ref_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count();
    }

    return 0;
  }

  [[nodiscard]] PROMPP(inline) bool non_atomic_is_unique() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count() == 1;
    }

    return true;
  }

  [[nodiscard]] PROMPP(inline) ControlBlock::ItemCounter items_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->items_count();
    }

    return 0;
  }

  PROMPP(inline) void set_items_count(ControlBlock::ItemCounter count) noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      block->set_items_count(count);
    }
  }

  [[nodiscard]] PROMPP(inline) T* get() const noexcept { return data_; }

  PROMPP(inline) void clear() noexcept {
    dec_ref_counter();
    data_ = nullptr;
  }

 private:
  T* data_{nullptr};

  PROMPP(inline) void non_atomic_reallocate(uint32_t size) noexcept {
    PROMPP(diagnostic, push)
    PROMPP(diagnostic, ignore, class_memaccess)
    auto control_block = static_cast<ControlBlock*>(Reallocator::reallocate(raw_memory(), kControlBlockSize + size * sizeof(T)));
    PROMPP(diagnostic, pop)

    if (control_block == nullptr) [[unlikely]] {
      std::abort();
    }

    if (data_ == nullptr) {
      std::construct_at(control_block);
    }

    data_ = reinterpret_cast<T*>(control_block + 1);
  }

  PROMPP(inline) void inc_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      ++block->atomic_ref_count();
    }
  }

  PROMPP(inline) void dec_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      if (block->ref_count() == 1) [[likely]] {
        destroy();
      } else {
        if (--block->atomic_ref_count() == 0) [[unlikely]] {
          destroy();
        }
      }
    }
  }

  PROMPP(inline) void destroy() noexcept {
    Reallocator::free(raw_memory());
    data_ = nullptr;
  }

  [[nodiscard]] PROMPP(inline) ControlBlock* control_block() noexcept { return static_cast<ControlBlock*>(raw_memory()); }
  [[nodiscard]] PROMPP(inline) const ControlBlock* control_block() const noexcept { return static_cast<ControlBlock*>(raw_memory()); }

  [[nodiscard]] PROMPP(inline) void* raw_memory() const noexcept { return data_ == nullptr ? nullptr : reinterpret_cast<ControlBlock*>(data_) - 1; }
};

template <class T, SharedPtrControlBlockInterface ControlBlockType, ReallocatorInterface Reallocator>
class SharedMemory : public GenericMemory<SharedMemory<T, ControlBlockType, Reallocator>, uint32_t, T> {
 public:
  using SizeType = uint32_t;
  using SharedPtr = BareBones::SharedPtr<T, ControlBlockType, Reallocator>;

  SharedMemory() = default;
  SharedMemory(const SharedMemory&) = default;
  SharedMemory(SharedMemory&& other) noexcept : data_(std::move(other.data_)), size_(std::exchange(other.size_, 0)) {}

  SharedMemory& operator=(const SharedMemory&) = default;
  PROMPP(inline) SharedMemory& operator=(SharedMemory&& other) noexcept {
    if (this != &other) [[likely]] {
      data_ = std::move(other.data_);
      size_ = std::exchange(other.size_, 0);
    }

    return *this;
  }

  [[nodiscard]] PROMPP(inline) SharedPtr::ControlBlock::ItemCounter items_count() const noexcept { return data_.items_count(); }
  PROMPP(inline) void set_items_count(SharedPtr::ControlBlock::ItemCounter count) noexcept { data_.set_items_count(count); }

  [[nodiscard]] PROMPP(inline) size_t allocated_memory() const noexcept {
    return size_ * sizeof(T) + (data_.get() != nullptr ? sizeof(SharedPtr::kControlBlockSize) : 0);
  }

  [[nodiscard]] PROMPP(inline) const SharedPtr& ptr() const noexcept { return data_; }

 protected:
  friend class GenericMemory<SharedMemory, SizeType, T>;

  [[nodiscard]] PROMPP(inline) SizeType get_size() const noexcept { return size_; }
  [[nodiscard]] PROMPP(inline) T* data() noexcept { return data_.get(); }
  [[nodiscard]] PROMPP(inline) const T* data() const noexcept { return data_.get(); }

  PROMPP(inline) void resize(SizeType new_size) noexcept {
    data_.reallocate(size_, new_size);
    size_ = new_size;
  }

 private:
  SharedPtr data_{};
  uint32_t size_{};
};

template <template <class> class ControlBlock, class T>
struct IsTriviallyReallocatable<Memory<ControlBlock, T>> : std::true_type {};

template <class T, SharedPtrControlBlockInterface ControlBlockType, ReallocatorInterface Reallocator>
struct IsTriviallyReallocatable<SharedMemory<T, ControlBlockType, Reallocator>> : std::true_type {};

template <template <class> class ControlBlock, class T>
struct IsZeroInitializable<Memory<ControlBlock, T>> : std::true_type {};

template <class T, SharedPtrControlBlockInterface ControlBlockType, ReallocatorInterface Reallocator>
struct IsZeroInitializable<SharedMemory<T, ControlBlockType, Reallocator>> : std::true_type {};

template <class T>
using MemoryWithItemCount = Memory<MemoryControlBlockWithItemCount, T>;

template <class T>
struct IsSharedMemory : std::false_type {};

template <class T, SharedPtrControlBlockInterface ControlBlockType, ReallocatorInterface Reallocator>
struct IsSharedMemory<SharedMemory<T, ControlBlockType, Reallocator>> : std::true_type {};

}  // namespace BareBones
