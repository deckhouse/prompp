#pragma once

#include "bare_bones/gorilla.h"
#include "traits.h"

namespace series_data::decoder {

template <std::unsigned_integral SampleCountType>
class GorillaDecodeIteratorGeneral : public DecodeIteratorTrait<GorillaDecodeIteratorGeneral<SampleCountType>> {
  using Base = DecodeIteratorTrait<GorillaDecodeIteratorGeneral>;

 public:
  GorillaDecodeIteratorGeneral(SampleCountType samples_count, const BareBones::BitSequenceReader& reader)
      : data_{.remaining_samples = samples_count, .reader{reader}, .decoder = {}} {
    if (data_.remaining_samples > 0) [[likely]] {
      data_.decoder.decode(data_.reader, data_.reader);
      update_sample();
    }
  }

  PROMPP(inline) GorillaDecodeIteratorGeneral& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP(inline) GorillaDecodeIteratorGeneral operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP(inline) double decoded_value() const noexcept { return data_.decoder.last_value(); }
  [[nodiscard]] PROMPP(inline) PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return data_.decoder.last_timestamp(); }

 protected:
  friend Base;

  struct Data {
    encoder::Sample sample{};
    SampleCountType remaining_samples{};
    BareBones::BitSequenceReader reader;
    BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder{};
  };

  static_assert(DecodeIteratorData<Data>);

  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  Data data_;

 private:
  PROMPP(inline) bool decode() noexcept {
    if (--data_.remaining_samples > 0) [[likely]] {
      data_.decoder.decode(data_.reader, data_.reader);
      return true;
    }

    return false;
  }

  PROMPP(inline) void update_sample() noexcept {
    data_.sample.value = data_.decoder.last_value();
    data_.sample.timestamp = data_.decoder.last_timestamp();
  }
};

}  // namespace series_data::decoder
