#pragma once

#include <span>

#include "bare_bones/preprocess.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/decoder.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::serialization {

class Deserializer {
 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool is_valid_buffer(std::span<const uint8_t> buffer) noexcept {
    if (buffer.size() < sizeof(uint32_t)) {
      return false;
    }

    const auto chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return buffer.size() >= sizeof(uint32_t) + chunks_count * sizeof(chunk::SerializedChunk);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static chunk::SerializedChunkSpan get_chunks(std::span<const uint8_t> buffer) noexcept {
    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return {reinterpret_cast<const chunk::SerializedChunk*>(buffer.data() + sizeof(uint32_t)), chunks_count};
  }
  [[nodiscard]] static decoder::UniversalDecodeIterator create_decode_iterator(std::span<const uint8_t> buffer, const chunk::SerializedChunk& chunk) {
    decoder::UniversalDecodeIterator iterator(std::in_place_type<decoder::ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0, false);
    Decoder::create_decode_iterator(buffer, chunk, [&iterator]<typename Iterator>(Iterator&& begin, auto&&) {
      iterator = decoder::UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
    });
    return iterator;
  }
  [[nodiscard]] static decoder::UniversalDecodeIterator create_decode_iterator(const chunk::SerializedChunkIterator::Data& chunk) {
    return create_decode_iterator(chunk.buffer(), chunk.chunk());
  }
  [[nodiscard]] static chunk::SerializedChunkIterator chunk_iterator(std::span<const uint8_t> buffer) noexcept {
    return chunk::SerializedChunkIterator(buffer);
  }

  explicit Deserializer(std::span<const uint8_t> buffer) : buffer_(buffer) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return is_valid_buffer(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::SerializedChunkSpan get_chunks() const noexcept { return get_chunks(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE decoder::UniversalDecodeIterator create_decode_iterator(const chunk::SerializedChunk& chunk) const {
    return create_decode_iterator(buffer_, chunk);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::SerializedChunkIterator begin() const noexcept { return chunk::SerializedChunkIterator(buffer_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

 private:
  const std::span<const uint8_t> buffer_;
};

}  // namespace series_data::serialization