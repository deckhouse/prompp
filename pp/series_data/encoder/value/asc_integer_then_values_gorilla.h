#pragma once

#include "asc_integer.h"
#include "values_gorilla.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED AscIntegerThenValuesGorillaEncoder : public ValuesGorillaEncoder {
 public:
  explicit AscIntegerThenValuesGorillaEncoder(AscIntegerEncoder&& asc_integer_encoder, double value)
      : ValuesGorillaEncoder(switch_to_values_gorilla(std::move(asc_integer_encoder)), value) {}

  using ValuesGorillaEncoder::allocated_memory;
  using ValuesGorillaEncoder::encode;
  using ValuesGorillaEncoder::finalize_stream;
  using ValuesGorillaEncoder::is_actual;
  using ValuesGorillaEncoder::last_value;
  using ValuesGorillaEncoder::stream;

 private:
  PROMPP_ALWAYS_INLINE static CompactBitSequence switch_to_values_gorilla(AscIntegerEncoder&& asc_integer_encoder) {
    auto stream = std::move(asc_integer_encoder).release_stream();
    AscIntegerEncoder::Encoder::write_switch_to_values_gorilla_mark(stream);
    return stream;
  }
};

}  // namespace series_data::encoder::value
