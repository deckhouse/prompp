#pragma once

#include "asc_integer.h"
#include "values_gorilla.h"

namespace series_data::encoder::value {

template <BareBones::ReallocatorInterface Reallocator>
class PROMPP_ATTRIBUTE_PACKED AscIntegerThenValuesGorillaEncoder : public ValuesGorillaEncoder<Reallocator> {
 public:
  using ValuesGorillaEncoder = series_data::encoder::value::ValuesGorillaEncoder<Reallocator>;

  explicit AscIntegerThenValuesGorillaEncoder(AscIntegerEncoder<Reallocator>&& asc_integer_encoder, double value)
      : ValuesGorillaEncoder(switch_to_values_gorilla(std::move(asc_integer_encoder)), value) {}

  using ValuesGorillaEncoder::allocated_memory;
  using ValuesGorillaEncoder::encode;
  using ValuesGorillaEncoder::finalize_stream;
  using ValuesGorillaEncoder::is_actual;
  using ValuesGorillaEncoder::last_value;
  using ValuesGorillaEncoder::stream;

 private:
  PROMPP_ALWAYS_INLINE static CompactBitSequence<Reallocator> switch_to_values_gorilla(AscIntegerEncoder<Reallocator>&& asc_integer_encoder) {
    auto stream = std::move(asc_integer_encoder).release_stream();
    AscIntegerEncoder<Reallocator>::Encoder::write_switch_to_values_gorilla_mark(stream);
    return stream;
  }
};

}  // namespace series_data::encoder::value
