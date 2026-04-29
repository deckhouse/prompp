#pragma once

#include "bare_bones/gorilla.h"
#include "bare_bones/iterator.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/chunkenc/bstream.h"
#include "prometheus/tsdb/chunkenc/xor.h"
#include "series_data/concepts.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
#include "series_data/encoder/bit_sequence.h"

namespace head {

template <class ChunkInfo>
concept ChunkInfoInterface = requires(ChunkInfo& info) {
  { info.interval } -> std::same_as<PromPP::Primitives::TimeInterval&>;
  { info.series_id } -> std::same_as<PromPP::Primitives::LabelSetID&>;
  { info.samples_count } -> std::same_as<uint8_t&>;
};

constexpr auto kUnlimitedLsIdBatchSize = std::numeric_limits<uint8_t>::max();

template <class LsIdSetIterator, class LsIdSetIteratorSentinel>
class ChunkRecoderIterator {
 public:
  using iterator_category = std::forward_iterator_tag;
  using value_type = series_data::DataStorage::SeriesChunkIterator::Data;
  using difference_type = ptrdiff_t;
  using pointer = value_type*;
  using reference = value_type&;

  using LabelSetID = PromPP::Primitives::LabelSetID;
  using IteratorSentinel = series_data::IteratorSentinel;

  ChunkRecoderIterator(LsIdSetIterator&& ls_id_iterator_,
                       LsIdSetIteratorSentinel&& ls_id_end_iterator,
                       uint32_t ls_id_batch_size,
                       const series_data::DataStorage* data_storage,
                       const PromPP::Primitives::TimeInterval time_interval)
      : time_interval_(time_interval),
        ls_id_iterator_(std::forward<LsIdSetIterator>(ls_id_iterator_), ls_id_batch_size),
        ls_id_end_iterator_(std::forward<LsIdSetIteratorSentinel>(ls_id_end_iterator)),
        chunk_iterator_(data_storage,
                        ls_id_iterator_ != ls_id_end_iterator_ ? static_cast<LabelSetID>(*ls_id_iterator_) : PromPP::Primitives::kInvalidLabelSetID) {
    advance_to_non_empty_chunk();
  }

  bool next_batch() noexcept {
    ls_id_iterator_.next_batch();

    if (*this != IteratorSentinel{}) {
      chunk_iterator_ = series_data::DataStorage::SeriesChunkIterator{chunk_iterator_->storage(), static_cast<LabelSetID>(*ls_id_iterator_)};
      advance_to_non_empty_chunk();
      return *this != IteratorSentinel{};
    }

    return false;
  }

  const value_type& operator*() const noexcept { return *chunk_iterator_; }
  const value_type* operator->() const noexcept { return chunk_iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE ChunkRecoderIterator& operator++() noexcept {
    advance_iterator();
    advance_to_non_empty_chunk();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ChunkRecoderIterator operator++(int) noexcept {
    const auto it = *this;
    ++*this;
    return it;
  }

  PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return ls_id_iterator_ == ls_id_end_iterator_; }

 private:
  const PromPP::Primitives::TimeInterval time_interval_;
  BareBones::iterator::BatchIterator<LsIdSetIterator, LsIdSetIteratorSentinel> ls_id_iterator_;
  [[no_unique_address]] LsIdSetIteratorSentinel ls_id_end_iterator_;
  series_data::DataStorage::SeriesChunkIterator chunk_iterator_;

  PROMPP_ALWAYS_INLINE void advance_iterator() noexcept {
    if (++chunk_iterator_ == IteratorSentinel{}) {
      if (++ls_id_iterator_ != ls_id_end_iterator_) {
        chunk_iterator_ = series_data::DataStorage::SeriesChunkIterator{chunk_iterator_->storage(), static_cast<LabelSetID>(*ls_id_iterator_)};
      }
    }
  }

  void advance_to_non_empty_chunk() noexcept {
    const auto chunk_is_empty = [this] PROMPP_LAMBDA_INLINE {
      if (this->chunk_is_empty()) {
        return true;
      }

      return !time_interval_.intersect({
          .min = series_data::Decoder::get_chunk_first_timestamp(**this),
          .max = series_data::Decoder::get_chunk_last_timestamp(**this),
      });
    };

    while (*this != IteratorSentinel{} && chunk_is_empty()) {
      advance_iterator();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool chunk_is_empty() const noexcept {
    return chunk_iterator_ == IteratorSentinel{} || chunk_iterator_->chunk().is_empty();
  }
};

template <class ChunkIterator>
class ChunkRecoder {
 public:
  explicit ChunkRecoder(ChunkIterator&& iterator, const PromPP::Primitives::TimeInterval& time_interval, PromPP::Primitives::Timestamp downsampling_ms)
      : iterator_(std::move(iterator)), time_interval_{time_interval}, downsampling_ms_{downsampling_ms} {}

  PROMPP_ALWAYS_INLINE ChunkIterator& chunk_iterator() noexcept { return iterator_; }

  void recode_next_chunk(ChunkInfoInterface auto& info) {
    reset_info(info);
    stream_.rewind();

    while (has_more_data()) {
      write_samples_count_placeholder();
      recode_chunk(info);

      ++iterator_;

      if (info.samples_count != 0) [[likely]] {
        write_samples_count(info.samples_count);
        break;
      }

      stream_.rewind();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> bytes() const noexcept { return stream_.bytes(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_data() const noexcept { return iterator_ != series_data::IteratorSentinel{}; }

 private:
  using Sample = series_data::encoder::Sample;
  using TimestampEncoder = PromPP::Prometheus::tsdb::chunkenc::TimestampEncoder;
  using ValuesEncoder = PromPP::Prometheus::tsdb::chunkenc::ValuesEncoder;
  using Encoder = BareBones::Encoding::Gorilla::StreamEncoder<TimestampEncoder, ValuesEncoder>;

  static constexpr uint32_t kSamplesCountSizeInBits = BareBones::Bit::to_bits(sizeof(uint16_t));
  static constexpr auto kMaxItemSizeInBits = TimestampEncoder::kMaxItemSizeInBits + ValuesEncoder::kMaxItemSizeInBits;
  static constexpr auto kMaxStreamSize = kSamplesCountSizeInBits + series_data::kSamplesPerChunkDefault * kMaxItemSizeInBits;

  ChunkIterator iterator_;
  PromPP::Prometheus::tsdb::chunkenc::FixedSizeBStream<series_data::encoder::kAllocationSizesTable> stream_{kMaxStreamSize};
  const PromPP::Primitives::TimeInterval time_interval_;
  const PromPP::Primitives::Timestamp downsampling_ms_;

  PROMPP_ALWAYS_INLINE static void reset_info(ChunkInfoInterface auto& info) noexcept {
    info.interval.reset(0, 0);
    info.samples_count = 0;
    info.series_id = PromPP::Primitives::kInvalidLabelSetID;
  }

  PROMPP_ALWAYS_INLINE void write_samples_count_placeholder() noexcept { stream_.write_bits(0, kSamplesCountSizeInBits); }
  PROMPP_ALWAYS_INLINE void write_samples_count(uint16_t samples_count) noexcept {
    *reinterpret_cast<uint16_t*>(stream_.raw_bytes()) = BareBones::Bit::be(samples_count);
  }

  void recode_chunk(ChunkInfoInterface auto& info) {
    Encoder encoder;

    //series_data::Decoder::create_decode_iterator(*iterator_, [&]<typename Iterator>(Iterator&& begin, auto&&) {
    //  if (downsampling_ms_ == series_data::decoder::decorator::kNoDownsampling) [[likely]] {
    //    recode_chunk(std::forward<Iterator>(begin), encoder, info);
    //  } else {
    //    recode_chunk(series_data::decoder::decorator::DownsamplingDecodeIterator(std::forward<Iterator>(begin), downsampling_ms_), encoder, info);
    //  }
    //});

    series_data::Decoder::create_decode_iterator(*iterator_, [&]<typename Iterator>(Iterator&& begin, auto&&) {
      begin.template seek<series_data::decoder::SeekKind::kNextStop>([&](int64_t timestamp, double value) {
        if (timestamp > time_interval_.max) [[unlikely]] {
          return series_data::decoder::SeekResult::kStop;
        }

        if (timestamp < time_interval_.min) [[unlikely]] {
          return series_data::decoder::SeekResult::kNext;
        }

        if (encoder.state().state == BareBones::Encoding::Gorilla::GorillaState::kFirstPoint) [[unlikely]] {
          info.interval.min = timestamp;
        }

        if constexpr (std::is_same_v<Iterator, series_data::decoder::ConstantDecodeIterator> ||
                      std::is_same_v<Iterator, series_data::decoder::TwoDoubleConstantDecodeIterator>) {
          encoder.encode_constant_value(timestamp, value, stream_, stream_);
        } else {
          encoder.encode(timestamp, value, stream_, stream_);
        }

        ++info.samples_count;
        return series_data::decoder::SeekResult::kNext;
      });
    });

    if (info.samples_count > 0) [[likely]] {
      info.interval.max = encoder.last_timestamp();
      info.series_id = iterator_->series_id();
    }
  }
};

}  // namespace head