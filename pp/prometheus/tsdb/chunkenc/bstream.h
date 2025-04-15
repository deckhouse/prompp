#pragma once

#include "bare_bones/bit_sequence.h"

namespace PromPP::Prometheus::tsdb::chunkenc {

PROMPP_ALWAYS_INLINE void write_bits(uint8_t* memory, uint64_t value, uint8_t nbits, uint8_t rest_of_bits_in_byte) noexcept {
  value <<= BareBones::Bit::kUint64Bits - nbits;
  *memory++ |= static_cast<uint8_t>(value >> (BareBones::Bit::kUint64Bits - rest_of_bits_in_byte));
  value <<= rest_of_bits_in_byte;
  *reinterpret_cast<uint64_t*>(memory) |= BareBones::Bit::be(value);
}

PROMPP_ALWAYS_INLINE void write_byte(uint8_t* memory, uint8_t byt, uint8_t unfilled_bits_in_byte, uint8_t rest_of_bits_in_byte) noexcept {
  *memory++ |= byt >> unfilled_bits_in_byte;
  *memory |= byt << rest_of_bits_in_byte;
}

PROMPP_ALWAYS_INLINE void write_single_bit(uint8_t* memory, uint8_t rest_of_bits_in_byte) noexcept {
  *memory |= 0b1u << (rest_of_bits_in_byte - 1);
}

template <std::array kAllocationSizesTable>
  requires std::is_same_v<typename decltype(kAllocationSizesTable)::value_type, BareBones::AllocationSize>
class BStream : public BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint64_t) + 1)> {
 public:
  void write_bits(uint64_t value, uint8_t nbits) noexcept {
    reserve_enough_memory_if_needed(nbits);
    chunkenc::write_bits(Base::template unfilled_memory<uint8_t>(), value, nbits, rest_of_bits_in_byte());
    size_in_bits_ += nbits;
  }

  PROMPP_ALWAYS_INLINE void write_byte(uint8_t byt) noexcept {
    reserve_enough_memory_if_needed();
    chunkenc::write_byte(Base::template unfilled_memory<uint8_t>(), byt, unfilled_bits_in_byte(), rest_of_bits_in_byte());
    size_in_bits_ += BareBones::Bit::kByteBits;
  }

  PROMPP_ALWAYS_INLINE void write_zero_bit() noexcept {
    reserve_enough_memory_if_needed();
    ++size_in_bits_;
  }

  PROMPP_ALWAYS_INLINE void write_single_bit() noexcept {
    reserve_enough_memory_if_needed();
    chunkenc::write_single_bit(Base::template unfilled_memory<uint8_t>(), rest_of_bits_in_byte());
    ++size_in_bits_;
  }

 private:
  using Base = BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint64_t) + 1)>;

  static constexpr auto kByteBits = BareBones::Bit::kByteBits;

  using Base::reserve_enough_memory_if_needed;
  using Base::size_in_bits_;
  using Base::unfilled_bits_in_byte;
  using Base::unfilled_memory;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t rest_of_bits_in_byte() const noexcept { return kByteBits - unfilled_bits_in_byte(); }
};

template <std::array kAllocationSizesTable>
  requires std::is_same_v<typename decltype(kAllocationSizesTable)::value_type, BareBones::AllocationSize>
class FixedSizeBStream : public BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint64_t) + 1)> {
 public:
  explicit FixedSizeBStream(uint32_t bits) { reserve_enough_memory_if_needed(bits); }

  void write_bits(uint64_t value, uint8_t nbits) noexcept {
    chunkenc::write_bits(Base::template unfilled_memory<uint8_t>(), value, nbits, rest_of_bits_in_byte());
    size_in_bits_ += nbits;
  }

  PROMPP_ALWAYS_INLINE void write_byte(uint8_t byt) noexcept {
    chunkenc::write_byte(Base::template unfilled_memory<uint8_t>(), byt, unfilled_bits_in_byte(), rest_of_bits_in_byte());
    size_in_bits_ += BareBones::Bit::kByteBits;
  }

  PROMPP_ALWAYS_INLINE void write_zero_bit() noexcept { ++size_in_bits_; }

  PROMPP_ALWAYS_INLINE void write_single_bit() noexcept {
    chunkenc::write_single_bit(Base::template unfilled_memory<uint8_t>(), rest_of_bits_in_byte());
    ++size_in_bits_;
  }

 private:
  using Base = BareBones::CompactBitSequenceBase<kAllocationSizesTable, BareBones::Bit::to_bits(sizeof(uint64_t) + 1)>;

  static constexpr auto kByteBits = BareBones::Bit::kByteBits;

  using Base::reserve_enough_memory_if_needed;
  using Base::size_in_bits_;
  using Base::unfilled_bits_in_byte;
  using Base::unfilled_memory;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t rest_of_bits_in_byte() const noexcept { return kByteBits - unfilled_bits_in_byte(); }
};

}  // namespace PromPP::Prometheus::tsdb::chunkenc