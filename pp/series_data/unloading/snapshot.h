#pragma once

#include <cstdint>
#include <span>

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::unloading {

class SnapshotChunkView {
 public:
  SnapshotChunkView(uint32_t ls_id, uint8_t chunk_id, std::span<const uint8_t> bytes) noexcept : ls_id_(ls_id), chunk_id_(chunk_id), bytes_(bytes) {}

  [[nodiscard]] uint32_t ls_id() const noexcept { return ls_id_; }
  [[nodiscard]] uint8_t chunk_id() const noexcept { return chunk_id_; }
  [[nodiscard]] std::span<const uint8_t> bytes() const noexcept { return bytes_; }

 private:
  uint32_t ls_id_{};
  uint8_t chunk_id_{};
  std::span<const uint8_t> bytes_{};
};

class SnapshotWriter {
 private:
  using EncodingChunkLengthSequence = BareBones::DeltaDeltaZigZagSequence;
  using EncodingChunkIDSequence =
      BareBones::RleSequence<BareBones::coding::Identity, BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>;

 public:
  template <class Stream, class Chunks>
  static void write_to(Stream& stream, const Chunks& chunks) noexcept {
    // `chunks` must be ordered by increasing, unique ls_id: the bitmap defines the reader's metadata order.
    BareBones::Bitset ls_id_bitmap;
    EncodingChunkLengthSequence chunk_length_sequence;
    EncodingChunkIDSequence chunk_id_sequence;
    uint32_t payload_byte_count = 0;

    for (const auto& chunk : chunks) {
      ls_id_bitmap.set(chunk.ls_id());
      chunk_length_sequence.push_back(chunk.bytes().size());
      chunk_id_sequence.push_back(chunk.chunk_id());
      payload_byte_count += chunk.bytes().size();
    }

    chunk_id_sequence.flush();
    chunk_length_sequence.flush();
    reserve_for_snapshot(stream, ls_id_bitmap, chunk_length_sequence, chunk_id_sequence, payload_byte_count);

    ls_id_bitmap.write_to(stream);
    chunk_length_sequence.store().write_to(stream);
    chunk_id_sequence.store().write_to(stream);
    write_chunk_payloads(stream, chunks);
    write_reader_padding(stream, payload_byte_count);
  }

 private:
  template <class Stream>
  static void reserve_for_snapshot(Stream& stream,
                                   const BareBones::Bitset& ls_id_bitmap,
                                   const EncodingChunkLengthSequence& chunk_length_sequence,
                                   const EncodingChunkIDSequence& chunk_id_sequence,
                                   uint32_t payload_byte_count) noexcept {
    if constexpr (BareBones::concepts::has_reserve<Stream>) {
      const uint32_t bitmap_byte_count = ls_id_bitmap.get_write_size();
      const uint32_t chunk_length_byte_count = chunk_length_sequence.store().get_write_size();
      const uint32_t chunk_id_byte_count = chunk_id_sequence.store().get_write_size();
      const uint32_t reader_padding_byte_count =
          payload_byte_count == 0 ? 0 : encoder::CompactBitSequence<BareBones::DefaultReallocator>::reserved_bytes_for_reader().size();

      stream.reserve(bitmap_byte_count + chunk_length_byte_count + chunk_id_byte_count + payload_byte_count + reader_padding_byte_count);
    }
  }

  template <class Stream, class Chunks>
  static void write_chunk_payloads(Stream& stream, const Chunks& chunks) noexcept {
    for (const auto& chunk : chunks) {
      const auto bytes = chunk.bytes();
      stream.write(reinterpret_cast<const char*>(bytes.data()), bytes.size());
    }
  }

  template <class Stream>
  static void write_reader_padding(Stream& stream, uint32_t payload_byte_count) noexcept {
    if (payload_byte_count != 0) {
      const auto& reserved_bytes = encoder::CompactBitSequence<BareBones::DefaultReallocator>::reserved_bytes_for_reader();
      stream.write(reserved_bytes.data(), reserved_bytes.size());
    }
  }
};

class SnapshotReader {
 public:
  // Requires a complete snapshot from a compatible SnapshotWriter.
  // The caller must preserve its bytes and verify storage integrity before this call.
  template <class Visitor>
  static void visit(std::span<const uint8_t> buffer, Visitor&& visitor) {
    auto ls_id = BareBones::Bitset::create_read_iterator(buffer);
    auto chunk_length = EncodingChunkLengthSequence::read_view(buffer).begin();
    auto chunk_id = EncodingChunkIDSequence::read_view(buffer).begin();
    const uint8_t* payload_begin = buffer.data();

    for (; ls_id != BareBones::Bitset::IteratorSentinel{}; ++ls_id, ++chunk_length, ++chunk_id) {
      const uint32_t payload_byte_count = *chunk_length;
      visitor(SnapshotChunkView{*ls_id, static_cast<uint8_t>(*chunk_id), {payload_begin, payload_byte_count}});
      payload_begin += payload_byte_count;
    }
  }

 private:
  using EncodingChunkLengthSequence = BareBones::DeltaDeltaZigZagSequence;
  using EncodingChunkIDSequence =
      BareBones::RleSequence<BareBones::coding::Identity, BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>;
};

}  // namespace series_data::unloading
