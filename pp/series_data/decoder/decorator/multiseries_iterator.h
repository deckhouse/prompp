#pragma once

#include "bare_bones/gorilla.h"

namespace series_data::decoder::decorator {

template <class Iterator, class SampleHandler>
class MultiSeriesIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit MultiSeriesIterator(BareBones::Vector<Iterator>&& iterators) : iterators_(std::move(iterators)) { advance(); }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE MultiSeriesIterator& operator++() {
    advance();
    return *this;
  }

  PROMPP_ALWAYS_INLINE MultiSeriesIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  encoder::Sample sample_;
  BareBones::Vector<Iterator> iterators_;

  void advance() {
    SampleHandler handler(sample_);
    for (auto& iterator : iterators_) {
      if (iterator != DecodeIteratorSentinel{}) [[likely]] {
        handler(*iterator++);
      }
    }

    handler.set_result();
  }
};

};  // namespace series_data::decoder::decorator
