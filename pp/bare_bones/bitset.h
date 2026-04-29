#pragma once

#include <cassert>
#ifdef __x86_64__
#include <x86intrin.h>
#endif
#ifdef __ARM_FEATURE_CRC32
#include <arm_acle.h>
#endif

#include <algorithm>
#include <atomic>
#include <bitset>
#include <numeric>
#include <ranges>

#include "bit.h"
#include "concepts.h"
#include "memory.h"
#include "streams.h"
#include "type_traits.h"

namespace BareBones {

template <ReallocatorInterface Reallocator = DefaultReallocator>
class GenericBitset {
  /**
   * Why??? Why another bitset??? Why no std::bitset?
   *
   * I've tested std::vector<bool> and roaring bitset, they both are significantly
   * slower:
   * - std::vector<bool> has no way of quickly iterating through set items
   * - roaring bitmap is not that quick if you can afford to hold the whole
   *   bitset in memory (including unset parts), which is the case
   */
  using Memory = BareBones::Memory<MemoryControlBlockWithItemCount, uint64_t, Reallocator>;
  Memory data_;

 public:
  void reserve(size_t size) noexcept {
    if (__builtin_expect(size > std::numeric_limits<uint32_t>::max(), false))
      std::abort();

    const auto size_in_uint64_elements = Bit::to_ceil_units<uint64_t>(size);

    if (size_in_uint64_elements <= data_.size()) {
      return;
    }

    data_.grow_to_fit_at_least_and_fill_with_zeros(size_in_uint64_elements);
  }

  void resize(size_t new_size) noexcept {
    reserve(new_size);

    // unset on downsize
    if (new_size < size()) {
      const uint64_t new_size_in_uint64_elements = (new_size + 63) >> 6;
      const uint64_t original_size_in_uint64_elements = (size() + 63) >> 6;
      std::memset(data_ + new_size_in_uint64_elements, 0, (original_size_in_uint64_elements - new_size_in_uint64_elements) << 3);
      data_[new_size >> 6] &= ~(0xFFFFFFFFFFFFFFFF << (new_size & 0x3F));
    }

    set_size(static_cast<uint32_t>(new_size));
  }

  // TODO shrink_to_fit

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return data_.control_block().items_count; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t capacity() const noexcept { return static_cast<size_t>(data_.size()) * 64; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept {
    return std::ranges::all_of(data_, [](const uint64_t v) { return v == 0; });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_set(uint32_t v) const noexcept { return v < size() && (data_[v >> 6] & (1ull << (v & 0x3F))) != 0; }

  PROMPP_ALWAYS_INLINE void set(uint32_t v) noexcept {
    if (v >= size()) [[unlikely]] {
      resize(v + 1);
    }

    data_[v >> 6] |= (1ull << (v & 0x3F));
  }

  PROMPP_ALWAYS_INLINE void set_atomic(uint32_t v) noexcept {
    assert(v < size());
    std::atomic_ref{data_[v >> 6]} |= (1ull << (v & 0x3F));
  }

  template <class It>
  PROMPP_ALWAYS_INLINE void set(It begin, It end) noexcept {
    for (auto it = begin; it != end; ++it) {
      set(*it);
    }
  }

  PROMPP_ALWAYS_INLINE void set(std::initializer_list<uint32_t> values) noexcept { set(values.begin(), values.end()); }

  PROMPP_ALWAYS_INLINE void reset(uint32_t v) noexcept {
    assert(v < size());
    data_[v >> 6] &= ~(1ull << (v & 0x3F));
  }

  PROMPP_ALWAYS_INLINE void reset_atomic(uint32_t v) noexcept {
    assert(v < size());
    std::atomic_ref{data_[v >> 6]} &= ~(1ull << (v & 0x3F));
  }

  template <class It>
  PROMPP_ALWAYS_INLINE void reset(It begin, It end) noexcept {
    for (auto it = begin; it != end; ++it) {
      reset(*it);
    }
  }

  PROMPP_ALWAYS_INLINE void reset(std::initializer_list<uint32_t> values) noexcept { reset(values.begin(), values.end()); }

  PROMPP_ALWAYS_INLINE bool operator[](uint32_t v) const noexcept {
    assert(v < size());
    return (data_[v >> 6] & (1ull << (v & 0x3F))) > 0;
  }

  void clear() noexcept {
    if (size() != 0) {
      const uint64_t size_in_uint64_elements = (size() + 63) >> 6;
      assert(size_in_uint64_elements <= data_.size());
      std::memset(data_, 0, size_in_uint64_elements << 3);
    }
    set_size(0);
  }

  class IteratorSentinel {};

  class Iterator {
    const uint64_t* data_{};

    uint32_t last_block_n_{};
    uint32_t block_n_{};
    uint64_t block_;
    uint32_t j_{64};

    PROMPP_ALWAYS_INLINE void next() noexcept {
      if (!block_ && block_n_ != last_block_n_) {
        while (++block_n_ != last_block_n_ && !data_[block_n_]) {
        }
        block_ = data_[block_n_];
      }

      j_ = std::countr_zero(block_);
      block_ &= ~(1ull << j_);
    }

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = uint32_t;
    using difference_type = std::ptrdiff_t;

    Iterator() = default;
    PROMPP_ALWAYS_INLINE explicit Iterator(const uint64_t* data, uint32_t size, uint32_t i) noexcept
        : data_(data), last_block_n_(size ? ((size - 1) >> 6) : 0), block_n_(i >> 6), j_(i & 0x3F) {
      block_ = size ? data_[block_n_] : 0;
      next();
    }

    PROMPP_ALWAYS_INLINE uint32_t operator*() const noexcept { return (block_n_ << 6) | j_; }
    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      next();
      return *this;
    }
    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const Iterator retval = *this;
      next();
      return retval;
    }
    PROMPP_ALWAYS_INLINE bool operator==(const Iterator& other) const noexcept { return block_n_ == other.block_n_ && j_ == other.j_; }
    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return block_n_ == last_block_n_ && j_ == 64; }
  };

  using const_iterator = Iterator;

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(data_, size(), 0); }
  [[nodiscard]] static PROMPP_ALWAYS_INLINE auto end() noexcept { return IteratorSentinel(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return data_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t popcount() const noexcept {
    return std::accumulate(data_.begin(), data_.end(), 0U, [](uint32_t popcount, uint64_t v) PROMPP_LAMBDA_INLINE { return popcount + std::popcount(v); });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_write_size() const noexcept {
    const uint32_t data_size_in_bytes = memory_size_in_bytes();
    return sizeof(data_size_in_bytes) + data_size_in_bytes;
  }

  template <OutputStream S>
  PROMPP_ALWAYS_INLINE void write_to(S& stream) const noexcept {
    const uint32_t data_size_in_bits = size();
    const uint32_t data_size_in_bytes = memory_size_in_bytes();

    if constexpr (BareBones::concepts::has_reserve<S>) {
      stream.reserve(sizeof(data_size_in_bits) + data_size_in_bytes);
    }

    stream.write(reinterpret_cast<const char*>(&data_size_in_bits), sizeof(data_size_in_bits));
    stream.write(reinterpret_cast<const char*>(data_.begin()), data_size_in_bytes);
  }

  static PROMPP_ALWAYS_INLINE Iterator create_read_iterator(std::span<const uint8_t>& buffer) noexcept {
    if (buffer.size() < sizeof(uint32_t)) [[unlikely]] {
      return Iterator{nullptr, 0, 0};
    }

    uint32_t bit_count = 0;
    std::memcpy(&bit_count, buffer.data(), sizeof(uint32_t));
    buffer = buffer.subspan(sizeof(uint32_t));

    const uint32_t uint64_count = BareBones::Bit::to_ceil_units<uint64_t>(bit_count);
    const uint32_t byte_count = uint64_count * sizeof(uint64_t);
    if (buffer.size() < byte_count) [[unlikely]] {
      return Iterator{nullptr, 0, 0};
    }

    const std::span bit_data(reinterpret_cast<const uint64_t*>(buffer.data()), uint64_count);
    buffer = buffer.subspan(byte_count);

    return Iterator(bit_data.data(), bit_count, 0);
  }

  [[nodiscard]] bool read_from(std::istream& stream) {
    uint32_t bit_count{};
    stream.read(reinterpret_cast<char*>(&bit_count), sizeof(uint32_t));
    if (stream.gcount() != sizeof(uint32_t)) [[unlikely]] {
      return false;
    }

    if (bit_count == 0) {
      return true;
    }

    resize(bit_count);
    const auto size_in_bytes = static_cast<std::streamsize>(memory_size_in_bytes(bit_count));
    stream.read(reinterpret_cast<char*>(data_.begin()), size_in_bytes);

    return stream.gcount() == size_in_bytes;
  }

 private:
  PROMPP_ALWAYS_INLINE void set_size(uint32_t new_size) noexcept { data_.control_block().items_count = new_size; }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE size_t memory_size_in_bytes(uint32_t bytes) noexcept {
    return Bit::to_ceil_units<uint64_t>(bytes) * sizeof(uint64_t);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t memory_size_in_bytes() const noexcept { return memory_size_in_bytes(size()); }
};

using Bitset = GenericBitset<DefaultReallocator>;

template <ReallocatorInterface Reallocator>
struct IsTriviallyReallocatable<GenericBitset<Reallocator>> : std::true_type {};

template <ReallocatorInterface Reallocator>
struct IsZeroInitializable<GenericBitset<Reallocator>> : std::true_type {};

}  // namespace BareBones

// GenericBitset::size() is the allocated bit width, not ranges::distance(begin, end) (set-bit
// count). GCC 14+ libstdc++ uses sized-range fast paths in std::ranges::equal; disable them.
namespace std::ranges {

template <BareBones::ReallocatorInterface R>
inline constexpr bool disable_sized_range<BareBones::GenericBitset<R>> = true;

}  // namespace std::ranges
