#pragma once

#include <type_traits>
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
  enum class Type {
    kConstant = 0,
    kTwoDoubleConstant,
    kAscInteger,
    kAscIntegerThenValuesGorilla,
    kValuesGorilla,
    kGorilla,
  };

  DECODE_ITERATOR_TYPE_TRAITS();

  UniversalDecodeIterator() : iterator_(std::in_place_type<ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0.0, false) {}

  template <class InPlaceType, class... Args>
  explicit UniversalDecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return reinterpret_cast<const encoder::Sample&>(iterator_); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return reinterpret_cast<const encoder::Sample*>(&iterator_); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const noexcept {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator& operator++() noexcept {
    std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE { ++iterator; }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE UniversalDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <SeekKind Kind, class SeekHandler>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    std::visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.template seek<Kind>(std::forward<SeekHandler>(handler)); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE void seek_to(PromPP::Primitives::Timestamp timestamp) {
    std::visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.seek_to(timestamp); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE void invalidate_sample() {
    std::visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.invalidate_sample(); }, iterator_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return static_cast<Type>(iterator_.index()); }

 private:
  using IteratorVariant = std::variant<ConstantDecodeIterator,
                                       TwoDoubleConstantDecodeIterator,
                                       AscIntegerDecodeIterator,
                                       AscIntegerThenValuesGorillaDecodeIterator,
                                       ValuesGorillaDecodeIterator,
                                       GorillaDecodeIterator>;

#ifdef __cpp_lib_is_pointer_interconvertible
  template <class Variant>
  struct SampleIsFirstIteratorsField;

  template <class... Iterators>
  struct SampleIsFirstIteratorsField<std::variant<Iterators...>> {
    template <class Iterator>
    struct SampleIsFirstIteratorField : Iterator {
      static constexpr auto value = std::is_pointer_interconvertible_with_class(&SampleIsFirstIteratorField::sample_);
    };

    static constexpr bool value = (SampleIsFirstIteratorField<Iterators>::value && ...);
  };

  static_assert(SampleIsFirstIteratorsField<IteratorVariant>::value, "each iterator must contain encoder::Sample as the first field");
#endif

  union VariantLayoutAssertHelper {
    IteratorVariant variant;
    const ConstantDecodeIterator field;
  };

  static constexpr auto kVariantLayoutAssertHelper = VariantLayoutAssertHelper{
      .variant = IteratorVariant(std::in_place_type<ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0, false),
  };
  static_assert(&std::get<ConstantDecodeIterator>(kVariantLayoutAssertHelper.variant) == &kVariantLayoutAssertHelper.field, "unexpected std::variant layout");

  IteratorVariant iterator_;
};

}  // namespace series_data::decoder
