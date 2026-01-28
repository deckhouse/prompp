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
  kUpdateSample = 0,
  kNext,
  kStop,
};

template <class Iterator>
concept Seekable = requires(Iterator iterator, const Iterator const_iterator) {
  { const_iterator.decoded_timestamp() } -> std::same_as<PromPP::Primitives::Timestamp>;
  { iterator.update_sample() };
  { iterator.decode() };
};

template <class Derived, std::unsigned_integral SampleCountType>
class DecodeIteratorTrait {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit DecodeIteratorTrait(SampleCountType count) : remaining_samples_{count} {}
  explicit DecodeIteratorTrait(double value, SampleCountType count) : sample_{.value = value}, remaining_samples_{count} {}
  explicit DecodeIteratorTrait(double value, SampleCountType count, bool last_stalenan)
      : sample_{.value = value}, remaining_samples_{count}, last_stalenan_{last_stalenan} {}

  const encoder::Sample& operator*() const noexcept { return sample_; }
  const encoder::Sample* operator->() const noexcept { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return remaining_samples_ == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SampleCountType remaining_samples() const noexcept { return remaining_samples_; }

  template <class SeekHandler>
    requires Seekable<Derived>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    if (remaining_samples_ == 0) [[unlikely]] {
      return;
    }

    do {
      if (const SeekResult result = handler(derived()->decoded_timestamp()); result == SeekResult::kUpdateSample) [[likely]] {
        derived()->update_sample();
      } else if (result == SeekResult::kStop) {
        break;
      }
    } while (derived()->decode());
  }

  PROMPP_ALWAYS_INLINE void invalidate() noexcept {
    remaining_samples_ = 0;
    sample_.timestamp = kInvalidTimestamp;
  }

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

  SeparatedTimestampValueDecodeIteratorTrait(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool last_stalenan)
      : Base(value, samples_count, last_stalenan), timestamp_decoder_(timestamp_reader) {
    if (Base::remaining_samples_ > 0) [[likely]] {
      Base::sample_.timestamp = timestamp_decoder_.decode();
    }
  }
  explicit SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), 0.0, false) {}
  SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), value, false) {}
  SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value, bool last_stalenan)
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