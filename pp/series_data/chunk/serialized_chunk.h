#pragma once

#include "bare_bones/vector.h"
#include "data_chunk.h"
#include "primitives/primitives.h"

namespace series_data::chunk {

struct PROMPP_ATTRIBUTE_PACKED SerializedChunk {
  PromPP::Primitives::LabelSetID label_set_id;
  EncodingState encoding_state{EncodingType::kUnknown, false};
  uint32_t values_offset{};
  uint32_t timestamps_offset{};

  explicit SerializedChunk(PromPP::Primitives::LabelSetID _label_set_id) : label_set_id(_label_set_id) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return encoding_state.encoding_type == EncodingType::kUnknown; }
  void set_offset(uint32_t offset) noexcept { values_offset = offset; }
  template <class T>
  void store_value_in_offset(T value) noexcept {
    values_offset = std::bit_cast<uint32_t>(value);
  }
};

using SerializedChunkList = BareBones::Vector<SerializedChunk>;

using SerializedChunkSpan = std::span<const SerializedChunk>;

class SerializedChunkIterator {
 public:
  class Data {
   public:
    Data(std::span<const uint8_t> buffer, SerializedChunkSpan chunks) : buffer_(buffer), chunk_iterator_(chunks.begin()), chunk_end_iterator_(chunks.end()) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const SerializedChunk& chunk() const noexcept { return *chunk_iterator_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> buffer() const noexcept { return buffer_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::LabelSetID series_id() const noexcept { return chunk_iterator_->label_set_id; }

   private:
    friend class SerializedChunkIterator;

    const std::span<const uint8_t> buffer_;
    SerializedChunkSpan::iterator chunk_iterator_;
    SerializedChunkSpan::iterator chunk_end_iterator_;

    PROMPP_ALWAYS_INLINE void next_value() noexcept { ++chunk_iterator_; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_value() const noexcept { return chunk_iterator_ != chunk_end_iterator_; }
  };

  using iterator_category = std::forward_iterator_tag;
  using value_type = Data;
  using difference_type = ptrdiff_t;
  using pointer = value_type*;
  using reference = value_type&;

  explicit SerializedChunkIterator(std::span<const uint8_t> buffer) : data_(buffer, get_chunks(buffer)) {}
  explicit SerializedChunkIterator(std::span<const uint8_t> buffer, SerializedChunkSpan chunks) : data_(buffer, chunks) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const Data& operator*() const noexcept { return data_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Data* operator->() const noexcept { return &data_; }

  PROMPP_ALWAYS_INLINE SerializedChunkIterator& operator++() noexcept {
    data_.next_value();
    return *this;
  }

  PROMPP_ALWAYS_INLINE SerializedChunkIterator operator++(int) noexcept {
    const auto it = *this;
    ++*this;
    return it;
  }

  PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return !data_.has_value(); }

 private:
  Data data_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SerializedChunkSpan get_chunks(std::span<const uint8_t> buffer) noexcept {
    uint32_t chunks_count = *reinterpret_cast<const uint32_t*>(buffer.data());
    return {reinterpret_cast<const SerializedChunk*>(buffer.data() + sizeof(uint32_t)), chunks_count};
  }
};

}  // namespace series_data::chunk