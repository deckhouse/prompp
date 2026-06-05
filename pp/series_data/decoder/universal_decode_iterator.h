#pragma once

#include <memory>

#include "asc_integer.h"
#include "asc_integer_then_values_gorilla.h"
#include "constant.h"
#include "gorilla.h"
#include "two_double_constant.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class UniversalDecodeIterator {
 public:
  enum class Type : uint8_t {
    kConstant = 0,
    kTwoDoubleConstant,
    kAscInteger,
    kAscIntegerThenValuesGorilla,
    kValuesGorilla,
    kGorilla,
  };

  DECODE_ITERATOR_TYPE_TRAITS();

#define DEFINE_CONSTRUCTOR(DecodeIterator, field, type)                                                                                                        \
  template <class... Args>                                                                                                                                     \
  explicit UniversalDecodeIterator(std::in_place_type_t<DecodeIterator>, Args&&... args) : iterator_{.field{std::forward<Args>(args)...}}, type_{Type::type} { \
    struct SampleIsFirstIteratorField : DecodeIterator {                                                                                                       \
      static_assert(decoder::DecodeIteratorData<decltype(SampleIsFirstIteratorField::data_)>,                                                                  \
                    #DecodeIterator "::data_ must comply with the DecodeIteratorData concept");                                                                \
    };                                                                                                                                                         \
  }

  DEFINE_CONSTRUCTOR(ConstantDecodeIterator, constant, kConstant)
  DEFINE_CONSTRUCTOR(TwoDoubleConstantDecodeIterator, two_double_constant, kTwoDoubleConstant)
  DEFINE_CONSTRUCTOR(AscIntegerDecodeIterator, asc_int, kAscInteger)
  DEFINE_CONSTRUCTOR(AscIntegerThenValuesGorillaDecodeIterator, asc_int_then_values, kAscIntegerThenValuesGorilla)
  DEFINE_CONSTRUCTOR(ValuesGorillaDecodeIterator, values_gorilla, kValuesGorilla)
  DEFINE_CONSTRUCTOR(GorillaDecodeIterator, gorilla, kGorilla)

#undef DEFINE_CONSTRUCTOR

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return iterator_.constant.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.constant.operator->(); }

  template <class Visitor>
  PROMPP_ALWAYS_INLINE auto visit(Visitor&& visitor) const noexcept {
    switch (type_) {
      case Type::kConstant: {
        return std::forward<Visitor>(visitor)(iterator_.constant);
      }

      case Type::kTwoDoubleConstant: {
        return std::forward<Visitor>(visitor)(iterator_.two_double_constant);
      }

      case Type::kAscInteger: {
        return std::forward<Visitor>(visitor)(iterator_.asc_int);
      }

      case Type::kAscIntegerThenValuesGorilla: {
        return std::forward<Visitor>(visitor)(iterator_.asc_int_then_values);
      }

      case Type::kValuesGorilla: {
        return std::forward<Visitor>(visitor)(iterator_.values_gorilla);
      }

      default: {
        return std::forward<Visitor>(visitor)(iterator_.gorilla);
      }
    }
  }

  template <class Visitor>
  PROMPP_ALWAYS_INLINE auto visit(Visitor&& visitor) noexcept {
    return const_cast<const UniversalDecodeIterator*>(this)->visit(
        [&]<typename Iterator>(const Iterator& iterator) PROMPP_LAMBDA_INLINE { return std::forward<Visitor>(visitor)(const_cast<Iterator&>(iterator)); });
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const noexcept {
    return visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; });
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator& operator++() noexcept {
    visit([](auto& iterator) PROMPP_LAMBDA_INLINE { ++iterator; });
    return *this;
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <SeekKind Kind, class SeekHandler>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.template seek<Kind>(std::forward<SeekHandler>(handler)); });
  }

  PROMPP_ALWAYS_INLINE void seek_to(PromPP::Primitives::Timestamp timestamp) {
    visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.seek_to(timestamp); });
  }

  PROMPP_ALWAYS_INLINE void invalidate_sample() {
    visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.invalidate_sample(); });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return type_; }

 private:
  union {
    ConstantDecodeIterator constant;
    TwoDoubleConstantDecodeIterator two_double_constant;
    AscIntegerDecodeIterator asc_int;
    AscIntegerThenValuesGorillaDecodeIterator asc_int_then_values;
    ValuesGorillaDecodeIterator values_gorilla;
    GorillaDecodeIterator gorilla;
  } iterator_;

  Type type_;
};

}  // namespace series_data::decoder
