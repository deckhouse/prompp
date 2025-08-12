#pragma once

#include <atomic>
#include <cstring>

#include "preprocess.h"
#include "type_traits.h"

namespace BareBones {

template <class DataType, class SizeType>
class AllocationSizeCalculator {
 public:
  static constexpr size_t kMinAllocationSize = 32;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t calculate(SizeType needed_size) noexcept {
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

template <class Derived, class SizeType, class T>
class GenericMemory {
 public:
  static_assert(IsTriviallyReallocatable<T>::value, "type parameter of this class should be trivially reallocatable");

  using value_type = T;
  using iterator = T*;
  using const_iterator = const T*;

  PROMPP_ALWAYS_INLINE void resize_to_fit_at_least(SizeType needed_size) noexcept { derived()->resize(get_allocation_size(needed_size)); }

  PROMPP_ALWAYS_INLINE void grow_to_fit_at_least(SizeType needed_size) noexcept {
    if (needed_size > size()) {
      resize_to_fit_at_least(needed_size);
    }
  }

  PROMPP_ALWAYS_INLINE void grow_to_fit_at_least_and_fill_with_zeros(SizeType needed_size) noexcept {
    if (needed_size > size()) {
      const auto old_size = size();
      resize_to_fit_at_least(needed_size);
      std::memset(begin() + old_size, 0, (size() - old_size) * sizeof(T));
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return size() == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType size() const noexcept { return derived()->get_size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* begin() const noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* begin() noexcept { return derived()->data(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* end() const noexcept { return derived()->data() + size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* end() noexcept { return derived()->data() + size(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP_ALWAYS_INLINE operator const T*() const noexcept { return begin(); }

  // NOLINTNEXTLINE(google-explicit-constructor)
  [[nodiscard]] PROMPP_ALWAYS_INLINE operator T*() noexcept { return begin(); }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static SizeType get_allocation_size(SizeType needed_size) noexcept {
    if constexpr (kIsUnitTestBuild || kIsBuildWithAsan) {
      // In unit tests or in build with asan we allocate only needed_size bytes. It helps us to debug memory access errors
      return needed_size;
    } else {
      return AllocationSizeCalculator<T, SizeType>::calculate(needed_size);
    }
  }

  PROMPP_ALWAYS_INLINE Derived* derived() noexcept { return static_cast<Derived*>(this); }
  PROMPP_ALWAYS_INLINE const Derived* derived() const noexcept { return static_cast<const Derived*>(this); }
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

  MemoryControlBlock() = default;
  MemoryControlBlock(const MemoryControlBlock& other) : data{nullptr}, data_size{other.data_size} {};
  MemoryControlBlock(MemoryControlBlock&& other) noexcept : data(std::exchange(other.data, nullptr)), data_size(std::exchange(other.data_size, 0)) {}

  MemoryControlBlock& operator=(const MemoryControlBlock&) { return *this; }
  PROMPP_ALWAYS_INLINE MemoryControlBlock& operator=(MemoryControlBlock&& other) noexcept {
    if (this != &other) [[likely]] {
      data = std::exchange(other.data, nullptr);
      data_size = std::exchange(other.data_size, 0);
    }

    return *this;
  }

  T* data{};
  SizeType data_size{};
};

template <class T>
struct MemoryControlBlockWithItemCount {
  using SizeType = uint32_t;

  MemoryControlBlockWithItemCount() = default;
  MemoryControlBlockWithItemCount(const MemoryControlBlockWithItemCount& other) : data{nullptr}, data_size{other.data_size}, items_count{other.items_count} {};
  MemoryControlBlockWithItemCount(MemoryControlBlockWithItemCount&& other) noexcept
      : data(std::exchange(other.data, nullptr)), data_size(std::exchange(other.data_size, 0)), items_count(std::exchange(other.items_count, 0)) {}

  MemoryControlBlockWithItemCount& operator=(const MemoryControlBlockWithItemCount& other) {
    if (this != &other) [[likely]] {
      items_count = other.items_count;
    }

    return *this;
  };

  PROMPP_ALWAYS_INLINE MemoryControlBlockWithItemCount& operator=(MemoryControlBlockWithItemCount&& other) noexcept {
    if (this != &other) [[likely]] {
      data = std::exchange(other.data, nullptr);
      data_size = std::exchange(other.data_size, 0);
      items_count = std::exchange(other.items_count, 0);
    }

    return *this;
  }

  T* data{};
  SizeType data_size{};
  SizeType items_count{};
};

template <template <class> class ControlBlock, class T>
  requires MemoryControlBlockInterface<ControlBlock, T>
class Memory : public GenericMemory<Memory<ControlBlock, T>, typename ControlBlock<T>::SizeType, T> {
 public:
  using SizeType = typename ControlBlock<T>::SizeType;

  PROMPP_ALWAYS_INLINE Memory() noexcept = default;
  PROMPP_ALWAYS_INLINE Memory(const Memory& o) noexcept : control_block_(o.control_block_) { copy(o); }
  PROMPP_ALWAYS_INLINE Memory(Memory&& o) noexcept = default;
  PROMPP_ALWAYS_INLINE ~Memory() noexcept { std::free(control_block_.data); }

  PROMPP_ALWAYS_INLINE Memory& operator=(const Memory& o) noexcept {
    if (this != &o) [[likely]] {
      copy(o);
    }

    control_block_ = o.control_block_;

    return *this;
  }

  PROMPP_ALWAYS_INLINE Memory& operator=(Memory&& o) noexcept {
    if (this != &o) [[likely]] {
      std::free(control_block_.data);
      control_block_ = std::move(o.control_block_);
    }

    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto& control_block() noexcept { return control_block_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& control_block() const noexcept { return control_block_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return control_block_.data_size * sizeof(T); }

 protected:
  friend class GenericMemory<Memory, SizeType, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType get_size() const noexcept { return control_block_.data_size; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* data() noexcept { return control_block_.data; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return control_block_.data; }

  PROMPP_ALWAYS_INLINE void resize(SizeType new_size) noexcept {
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    control_block_.data = static_cast<T*>(std::realloc(control_block_.data, new_size * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)

    if (control_block_.data == nullptr && new_size > 0) [[unlikely]] {
      std::abort();
    }

    control_block_.data_size = new_size;
  }

 private:
  ControlBlock<T> control_block_;

  PROMPP_ALWAYS_INLINE void copy(const Memory& o) noexcept {
    static_assert(IsTriviallyCopyable<T>::value, "it's not allowed to copy memory for non trivially copyable types");

    resize(o.control_block_.data_size);

    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    std::memcpy(control_block_.data, o.control_block_.data, control_block_.data_size * sizeof(T));
    PRAGMA_DIAGNOSTIC(pop)
  }
};

template <class Reallocator>
concept ReallocatorInterface = requires(Reallocator reallocator, void* memory) {
  { Reallocator::reallocate(memory, size_t()) } -> std::same_as<void*>;
  { Reallocator::free(memory) } -> std::same_as<void>;
};

struct DefaultReallocator {
  PROMPP_ALWAYS_INLINE static void* reallocate(void* memory, size_t size) { return std::realloc(memory, size); }
  PROMPP_ALWAYS_INLINE static void free(void* memory) { return std::free(memory); }
};

template <class T, ReallocatorInterface Reallocator>
class SharedPtr {
 public:
  using RefCounter = uint32_t;
  using ItemCounter = uint32_t;
  using AtomicRefCounter = std::atomic_ref<RefCounter>;

  struct ControlBlock {
    RefCounter ref_count{1};
    ItemCounter constructed_item_count{};

    [[nodiscard]] PROMPP_ALWAYS_INLINE AtomicRefCounter atomic_ref_count() noexcept { return AtomicRefCounter(ref_count); }
  };

  static constexpr uint32_t kControlBlockSize = sizeof(ControlBlock);

  SharedPtr() = default;
  explicit PROMPP_ALWAYS_INLINE SharedPtr(uint32_t size, ItemCounter constructed_item_count = 0) : data_(nullptr) {
    non_atomic_reallocate(size);
    set_constructed_item_count(constructed_item_count);
  }
  PROMPP_ALWAYS_INLINE SharedPtr(const SharedPtr& other) noexcept : data_(other.data_) { inc_ref_counter(); }
  SharedPtr(SharedPtr&& other) noexcept : data_(std::exchange(other.data_, nullptr)) {}

  PROMPP_ALWAYS_INLINE ~SharedPtr() { dec_ref_counter(); }

  PROMPP_ALWAYS_INLINE SharedPtr& operator=(const SharedPtr& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = other.data_;
      inc_ref_counter();
    }

    return *this;
  }

  PROMPP_ALWAYS_INLINE SharedPtr& operator=(SharedPtr&& other) noexcept {
    if (this != &other) [[likely]] {
      dec_ref_counter();
      data_ = std::exchange(other.data_, nullptr);
    }

    return *this;
  }

  PROMPP_ALWAYS_INLINE friend void swap(SharedPtr& a, SharedPtr& b) noexcept { std::swap(a.data_, b.data_); }

  PROMPP_ALWAYS_INLINE void non_atomic_reallocate(uint32_t size) noexcept {
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
    auto control_block = static_cast<ControlBlock*>(Reallocator::reallocate(raw_memory(), kControlBlockSize + size * sizeof(T)));
    PRAGMA_DIAGNOSTIC(pop)

    if (data_ == nullptr) [[likely]] {
      std::construct_at(control_block);
    } else {
      control_block->ref_count = 1;
    }

    data_ = reinterpret_cast<T*>(control_block + 1);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE RefCounter non_atomic_ref_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count;
    }

    return 0;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool non_atomic_is_unique() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->ref_count == 1;
    }

    return true;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE ItemCounter constructed_item_count() const noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      return block->constructed_item_count;
    }

    return 0;
  }

  PROMPP_ALWAYS_INLINE void set_constructed_item_count(ItemCounter count) noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      block->constructed_item_count = count;
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE T* get() const noexcept { return data_; }

  PROMPP_ALWAYS_INLINE void swap(SharedPtr& other) noexcept { std::swap(data_, other.data_); }

 private:
  T* data_{nullptr};

  PROMPP_ALWAYS_INLINE void inc_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      ++block->atomic_ref_count();
    }
  }

  PROMPP_ALWAYS_INLINE void dec_ref_counter() noexcept {
    if (auto block = control_block(); block != nullptr) [[likely]] {
      if (block->ref_count == 1) [[likely]] {
        destroy();
      } else {
        --block->atomic_ref_count();
      }
    }
  }

  PROMPP_ALWAYS_INLINE void destroy() noexcept {
    destroy_constructed_items();
    Reallocator::free(raw_memory());
    data_ = nullptr;
  }

  PROMPP_ALWAYS_INLINE void destroy_constructed_items() noexcept {
    for (T *it = reinterpret_cast<T*>(data_), *end = it + control_block()->constructed_item_count; it != end; ++it) {
      std::destroy_at(it);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE ControlBlock* control_block() noexcept { return static_cast<ControlBlock*>(raw_memory()); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const ControlBlock* control_block() const noexcept { return static_cast<ControlBlock*>(raw_memory()); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE void* raw_memory() const noexcept { return data_ == nullptr ? nullptr : reinterpret_cast<ControlBlock*>(data_) - 1; }
};

template <class T, ReallocatorInterface Reallocator>
class SharedMemory : public GenericMemory<SharedMemory<T, Reallocator>, uint32_t, T> {
 public:
  using SizeType = uint32_t;
  using SharedPtr = BareBones::SharedPtr<T, Reallocator>;

  SharedMemory() = default;
  SharedMemory(const SharedMemory&) = default;
  SharedMemory(SharedMemory&& other) noexcept : data_(std::move(other.data_)), size_(std::exchange(other.size_, 0)) {}

  SharedMemory& operator=(const SharedMemory&) = default;
  PROMPP_ALWAYS_INLINE SharedMemory& operator=(SharedMemory&& other) noexcept {
    if (this != &other) [[likely]] {
      data_ = std::move(other.data_);
      size_ = std::exchange(other.size_, 0);
    }

    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE typename SharedPtr::ItemCounter constructed_item_count() const noexcept { return data_.constructed_item_count(); }
  PROMPP_ALWAYS_INLINE void set_constructed_item_count(typename SharedPtr::ItemCounter count) noexcept { data_.set_constructed_item_count(count); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return size_ * sizeof(T) + (data_.get() != nullptr ? sizeof(SharedPtr::kControlBlockSize) : 0);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SharedPtr& ptr() const noexcept { return data_; }

 protected:
  friend class GenericMemory<SharedMemory, SizeType, T>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE SizeType get_size() const noexcept { return size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE T* data() noexcept { return data_.get(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const T* data() const noexcept { return data_.get(); }

  PROMPP_ALWAYS_INLINE void resize(SizeType new_size) noexcept {
    if (data_.non_atomic_is_unique()) [[likely]] {
      data_.non_atomic_reallocate(new_size);
    } else {
      SharedPtr new_data(new_size, constructed_item_count());
      PRAGMA_DIAGNOSTIC(push)
      PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_CLASS_MEMACCESS)
      std::memcpy(new_data.get(), data_.get(), size_ * sizeof(T));
      PRAGMA_DIAGNOSTIC(pop)
      swap(data_, new_data);
    }

    size_ = new_size;
  }

 private:
  SharedPtr data_{};
  uint32_t size_{};
};

template <template <class> class ControlBlock, class T>
struct IsTriviallyReallocatable<Memory<ControlBlock, T>> : std::true_type {};

template <class T, ReallocatorInterface Reallocator>
struct IsTriviallyReallocatable<SharedMemory<T, Reallocator>> : std::true_type {};

template <template <class> class ControlBlock, class T>
struct IsZeroInitializable<Memory<ControlBlock, T>> : std::true_type {};

template <class T, ReallocatorInterface Reallocator>
struct IsZeroInitializable<SharedMemory<T, Reallocator>> : std::true_type {};

template <class T>
using MemoryWithItemCount = Memory<MemoryControlBlockWithItemCount, T>;

template <class T>
struct IsSharedMemory : std::false_type {};

template <class T, ReallocatorInterface Reallocator>
struct IsSharedMemory<SharedMemory<T, Reallocator>> : std::true_type {};

}  // namespace BareBones
