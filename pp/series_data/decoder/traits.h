#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/sample.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data::decoder {

static constexpr auto kInvalidTimestamp = std::numeric_limits<PromPP::Primitives::Timestamp>::min();

class DecodeIteratorSentinel {};

#define DECODE_ITERATOR_TYPE_TRAITS()                  \
  using iterator_category = std::forward_iterator_tag; \
  using value_type = ::series_data::encoder::Sample;   \
  using difference_type = ptrdiff_t;                   \
  using pointer = value_type*;                         \
  using reference = value_type&

enum class SeekResult : uint8_t {
  kUpdateSample = 1,
  kNext,
  kStop,
  kUpdateSampleNextAndStop,
};

enum class SeekKind : uint8_t {
  kNextStop = static_cast<uint8_t>(SeekResult::kNext) | static_cast<uint8_t>(SeekResult::kStop),
  kAll = static_cast<uint8_t>(SeekResult::kUpdateSample) | static_cast<uint8_t>(SeekResult::kNext) | static_cast<uint8_t>(SeekResult::kStop) |
         static_cast<uint8_t>(SeekResult::kUpdateSampleNextAndStop),
};

template <class Iterator>
concept Seekable = requires(Iterator iterator, const Iterator const_iterator) {
  { const_iterator.decoded_timestamp() } -> std::same_as<PromPP::Primitives::Timestamp>;
  { const_iterator.decoded_value() } -> std::same_as<double>;
  { iterator.update_sample() };
  { iterator.decode() };
};

template <class SeekHandler>
concept SampleSeekHandler = std::is_invocable_v<SeekHandler, PromPP::Primitives::Timestamp, double>;

template <class Derived, std::unsigned_integral SampleCountType>
class DecodeIteratorTrait {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit constexpr DecodeIteratorTrait(SampleCountType count) : remaining_samples_{count} {}
  explicit constexpr DecodeIteratorTrait(double value, SampleCountType count) : sample_{.value = value}, remaining_samples_{count} {}
  explicit constexpr DecodeIteratorTrait(double value, SampleCountType count, bool last_stalenan)
      : sample_{.value = value}, remaining_samples_{count}, last_stalenan_{last_stalenan} {}

  const encoder::Sample& operator*() const noexcept { return sample_; }
  const encoder::Sample* operator->() const noexcept { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return remaining_samples_ == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SampleCountType remaining_samples() const noexcept { return remaining_samples_; }

  template <SeekKind Kind, class SeekHandler>
    requires Seekable<Derived>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    static constexpr auto has_kind = [](SeekResult operation) { return static_cast<uint8_t>(Kind) & static_cast<uint8_t>(operation); };

    if (remaining_samples_ == 0) [[unlikely]] {
      return;
    }

    do {
      SeekResult result;
      if constexpr (SampleSeekHandler<SeekHandler>) {
        result = handler(derived()->decoded_timestamp(), derived()->decoded_value());
      } else {
        result = handler(derived()->decoded_timestamp());
      }

      assert(has_kind(result));

      if (has_kind(SeekResult::kUpdateSample) && result == SeekResult::kUpdateSample) [[likely]] {
        derived()->update_sample();
      } else if (has_kind(SeekResult::kStop) && result == SeekResult::kStop) {
        break;
      } else if (has_kind(SeekResult::kUpdateSampleNextAndStop) && result == SeekResult::kUpdateSampleNextAndStop) {
        derived()->update_sample();
        derived()->decode();
        break;
      }
    } while (derived()->decode());
  }

  PROMPP_ALWAYS_INLINE void seek_to(PromPP::Primitives::Timestamp timestamp) {
    if (remaining_samples_ == 0) [[unlikely]] {
      return;
    }

    while (derived()->decoded_timestamp() < timestamp) {
      if (!derived()->decode()) [[unlikely]] {
        return;
      }
    }

    derived()->update_sample();
  }

  PROMPP_ALWAYS_INLINE void invalidate_sample() noexcept { sample_.timestamp = kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE void set(const encoder::Sample& sample) noexcept { sample_ = sample; }

 protected:
  encoder::Sample sample_;
  SampleCountType remaining_samples_{};
  bool last_stalenan_{false};

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE Derived* derived() noexcept { return static_cast<Derived*>(this); }
};

template <class Derived>
class SeparatedTimestampValueDecodeIteratorTrait : public DecodeIteratorTrait<Derived, uint8_t> {
 public:
  using Base = DecodeIteratorTrait<Derived, uint8_t>;

  constexpr SeparatedTimestampValueDecodeIteratorTrait(uint8_t samples_count,
                                                       const BareBones::BitSequenceReader& timestamp_reader,
                                                       double value,
                                                       bool last_stalenan)
      : Base(value, samples_count, last_stalenan), timestamp_decoder_(timestamp_reader) {
    if (Base::remaining_samples_ > 0) [[likely]] {
      Base::sample_.timestamp = timestamp_decoder_.decode();
    }
  }

  template <class BitSequenceWithItemsCount>
  explicit SeparatedTimestampValueDecodeIteratorTrait(const BitSequenceWithItemsCount& timestamp_stream)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), 0.0, false) {}

  template <class BitSequenceWithItemsCount>
  SeparatedTimestampValueDecodeIteratorTrait(const BitSequenceWithItemsCount& timestamp_stream, double value)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), value, false) {}

  template <class BitSequenceWithItemsCount>
  SeparatedTimestampValueDecodeIteratorTrait(const BitSequenceWithItemsCount& timestamp_stream, double value, bool last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), value, last_stalenan) {}

 protected:
  friend class DecodeIteratorTrait<Derived, uint8_t>;

  encoder::timestamp::TimestampDecoder timestamp_decoder_;

  PROMPP_ALWAYS_INLINE bool decode_timestamp() noexcept {
    if (--Base::remaining_samples_ > 0) [[likely]] {
      std::ignore = timestamp_decoder_.decode();
      return true;
    }

    return false;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return timestamp_decoder_.timestamp(); }
};

}  // namespace series_data::decoder
