#pragma once

#include <variant>

#include "asc_integer.h"
#include "asc_integer_then_values_gorilla.h"
#include "constant.h"
#include "gorilla.h"
#include "two_double_constant.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class UniversalDecodeIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  UniversalDecodeIterator() : iterator_(std::in_place_type<ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0.0, false) {}

  template <class InPlaceType, class... Args>
  explicit UniversalDecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const& { return *iterator; }, iterator_);
  }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const* { return iterator.operator->(); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator& operator++() {
    std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE { ++iterator; }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <class SeekHandler>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    std::visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.seek(std::forward<SeekHandler>(handler)); }, iterator_);
  }

 private:
  using IteratorVariant = std::variant<ConstantDecodeIterator,
                                       TwoDoubleConstantDecodeIterator,
                                       AscIntegerDecodeIterator,
                                       AscIntegerThenValuesGorillaDecodeIterator,
                                       ValuesGorillaDecodeIterator,
                                       GorillaDecodeIterator>;

  IteratorVariant iterator_;
};

}  // namespace series_data::decoder
