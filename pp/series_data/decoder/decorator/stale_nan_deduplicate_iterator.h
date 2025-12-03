#pragma once

#include "bare_bones/gorilla.h"
#include "series_data/decoder/traits.h"

namespace series_data::decoder::decorator {

template <class DecodeIterator, class DecodeIteratorSentinel>
class StaleNanDeduplicateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  StaleNanDeduplicateIterator(DecodeIterator&& iterator, DecodeIteratorSentinel&& end) : iterator_(std::move(iterator)), iterator_end_(std::move(end)) {}

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return *iterator_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return iterator_ == iterator_end_; }

  PROMPP_ALWAYS_INLINE StaleNanDeduplicateIterator& operator++() noexcept {
    advance_to_next_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE StaleNanDeduplicateIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  DecodeIterator iterator_;
  [[no_unique_address]] DecodeIteratorSentinel iterator_end_;

  PROMPP_ALWAYS_INLINE void advance_to_next_sample() noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(iterator_->value)) [[unlikely]] {
      skip_stale_nans();
    } else {
      ++iterator_;
    }
  }

  PROMPP_ALWAYS_INLINE void skip_stale_nans() noexcept {
    while (++iterator_ != iterator_end_) {
      if (!BareBones::Encoding::Gorilla::isstalenan(iterator_->value)) {
        break;
      }
    }
  }
};

}  // namespace series_data::decoder::decorator