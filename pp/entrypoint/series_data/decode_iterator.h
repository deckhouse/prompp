#pragma once

#include <variant>

#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace entrypoint::series_data {

template <class Iterator>
concept invalidatable = requires(Iterator iterator) {
  { iterator.invalidate() };
};

class DecodeIterator {
 public:
  using UniversalDecodeIterator = ::series_data::decoder::UniversalDecodeIterator;
  using DownsamplingIterator = ::series_data::decoder::decorator::DownsamplingDecodeIterator<UniversalDecodeIterator>;
  using DecodeIteratorSentinel = ::series_data::decoder::DecodeIteratorSentinel;
  using IteratorVariant = std::variant<UniversalDecodeIterator, DownsamplingIterator>;

  DECODE_ITERATOR_TYPE_TRAITS();

  template <class InPlaceType, class... Args>
  explicit DecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE DecodeIterator& operator=(UniversalDecodeIterator&& it) {
    std::visit([&it](auto& iterator) PROMPP_LAMBDA_INLINE { iterator = std::move(it); }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample& operator*() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const& { return *iterator; }, iterator_);
  }
  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample* operator->() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const* { return iterator.operator->(); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE DecodeIterator& operator++() {
    std::visit(
        []<typename Iterator>(Iterator& iterator) PROMPP_LAMBDA_INLINE {
          ++iterator;

          if constexpr (invalidatable<Iterator>) {
            if (iterator == DecodeIteratorSentinel{}) [[unlikely]] {
              iterator.invalidate();
            }
          }
        },
        iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE DecodeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  IteratorVariant iterator_;
};

}  // namespace entrypoint::series_data