#pragma once

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
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
  kNext = 2,
  kStop = 4,
  kNextAndStop = 8,
  kUpdateSampleNextAndStop = 16,
};

enum class SeekKind : uint8_t {
  kUpdateSample_Stop = BareBones::build_bitmask<uint8_t, SeekResult::kUpdateSample, SeekResult::kStop>(),
  kNext_Stop = BareBones::build_bitmask<uint8_t, SeekResult::kNext, SeekResult::kStop>(),
  kNext_Stop_NextAndStop = BareBones::build_bitmask<uint8_t, SeekResult::kNext, SeekResult::kStop, SeekResult::kNextAndStop>(),
  kUpdateSample_Next_Stop = BareBones::build_bitmask<uint8_t, SeekResult::kUpdateSample, SeekResult::kNext, SeekResult::kStop>(),
  kAll = BareBones::
      build_bitmask<uint8_t, SeekResult::kUpdateSample, SeekResult::kNext, SeekResult::kStop, SeekResult::kNextAndStop, SeekResult::kUpdateSampleNextAndStop>(),
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

template <class Data>
concept DecodeIteratorData = requires(Data data) {
  requires std::same_as<encoder::Sample, decltype(data.sample)>;

  data.remaining_samples;

#ifdef __cpp_lib_is_pointer_interconvertible
  requires std::is_pointer_interconvertible_with_class(&Data::sample);
#endif
};

template <class Derived>
class DecodeIteratorTrait {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  const encoder::Sample& operator*() const noexcept { return derived()->data_.sample; }
  const encoder::Sample* operator->() const noexcept { return &derived()->data_.sample; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return derived()->data_.remaining_samples == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto remaining_samples() const noexcept { return derived()->data_.remaining_samples; }

  template <SeekKind Kind, class SeekHandler>
    requires Seekable<Derived>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    static constexpr auto has_kind = [](SeekResult operation) { return static_cast<uint8_t>(Kind) & static_cast<uint8_t>(operation); };

    if (derived()->data_.remaining_samples == 0) [[unlikely]] {
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
      } else if (has_kind(SeekResult::kNextAndStop) && result == SeekResult::kNextAndStop) {
        derived()->decode();
        break;
      } else if (has_kind(SeekResult::kUpdateSampleNextAndStop) && result == SeekResult::kUpdateSampleNextAndStop) {
        derived()->update_sample();
        derived()->decode();
        break;
      }
    } while (derived()->decode());
  }

  PROMPP_ALWAYS_INLINE void seek_to(PromPP::Primitives::Timestamp timestamp) {
    if (derived()->data_.remaining_samples == 0) [[unlikely]] {
      return;
    }

    while (derived()->decoded_timestamp() < timestamp) {
      if (!derived()->decode()) [[unlikely]] {
        return;
      }
    }

    derived()->update_sample();
  }

  PROMPP_ALWAYS_INLINE void invalidate_sample() noexcept { derived()->data_.sample.timestamp = kInvalidTimestamp; }

 protected:
  [[nodiscard]] PROMPP_ALWAYS_INLINE Derived* derived() noexcept { return static_cast<Derived*>(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Derived* derived() const noexcept { return static_cast<const Derived*>(this); }
};

template <class Data>
concept DecodeIteratorDataWithTimestampDecoder = requires(Data data) {
  requires DecodeIteratorData<Data>;

  requires std::same_as<encoder::timestamp::TimestampDecoder, decltype(data.timestamp_decoder)>;
};

template <DecodeIteratorDataWithTimestampDecoder Data>
PROMPP_ALWAYS_INLINE bool decode_timestamp(Data& data) noexcept {
  if (--data.remaining_samples > 0) [[likely]] {
    std::ignore = data.timestamp_decoder.decode();
    return true;
  }

  return false;
}

}  // namespace series_data::decoder
