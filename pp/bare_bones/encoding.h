#pragma once

#include <iterator>
#include <limits>
#include <type_traits>
#include <utility>

#include <scope_exit.h>

#include "bit_sequence.h"
#include "exception.h"
#include "preprocess.h"
#include "stream_v_byte.h"
#include "streams.h"
#include "zigzag.h"

namespace BareBones {
namespace Encoding {
template <class Container>
class RLEBackend {
 public:
  using DataSequence = Container;

  class Encoder {
    using value_type = typename DataSequence::value_type;

    value_type count_ = std::numeric_limits<value_type>::max();
    value_type last_;

   public:
    PROMPP_ALWAYS_INLINE Encoder() noexcept = default;
    Encoder(const Encoder&) = delete;
    Encoder& operator=(const Encoder&) = delete;

    PROMPP_ALWAYS_INLINE Encoder(Encoder&& o) noexcept : count_(o.count_), last_(o.last_) { o.clear(); }

    PROMPP_ALWAYS_INLINE Encoder& operator=(Encoder&& o) noexcept {
      count_ = o.count_;
      last_ = o.last_;
      o.clear();
      return *this;
    }

    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      // assume last_ = val on first call
      if (count_ == std::numeric_limits<value_type>::max())
        last_ = val;

      if (val == last_) {
        ++count_;

        // check for overflow
        if (count_ == std::numeric_limits<value_type>::max() - 1) {
          *i++ = last_;
          *i++ = count_;
          count_ = 0;
        }
      } else {
        *i++ = last_;
        *i++ = count_;
        last_ = val;
        count_ = 0;  // use 0 to encode 1 occurrence
      }
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept { count_ = std::numeric_limits<value_type>::max(); }
    PROMPP_ALWAYS_INLINE bool empty() noexcept { return count_ == std::numeric_limits<value_type>::max(); }

    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void flush(IteratorType& i) noexcept {
      if (count_ != std::numeric_limits<value_type>::max()) {
        *i++ = last_;
        *i++ = count_;
        clear();
      }
    }

    PROMPP_ALWAYS_INLINE value_type count() const noexcept { return count_; }
    PROMPP_ALWAYS_INLINE value_type last() const noexcept { return last_; }
  };

  class Decoder {
    using value_type = typename DataSequence::value_type;

    value_type count_ = 0;
    value_type last_;
    bool decoding_from_encoder_buffer_ = false;

   public:
    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE Decoder(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      next(begin, end, encoder);
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE value_type decode(const IteratorType&, const IteratorSentinelType&, const Encoder&) const noexcept {
      return last_;
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      // range check
      assert(count_ != std::numeric_limits<value_type>::max());

      --count_;

      if (count_ == std::numeric_limits<value_type>::max()) {
        if (begin != end) {
          last_ = *begin++;
          assert(begin != end);

          if (__builtin_expect(begin == end, false))
            // that's not a normal situation, but we allow one missing count at the end
            // and treat it as if it would have been zero to make all the code exceptions free
            count_ = 0;
          else
            count_ = *begin++;
        } else if (!decoding_from_encoder_buffer_) {
          last_ = encoder.last();
          count_ = encoder.count();
          decoding_from_encoder_buffer_ = true;
        }
      }
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE bool is_finished(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return begin == end && count_ == std::numeric_limits<value_type>::max() &&
             (decoding_from_encoder_buffer_ || encoder.count() == std::numeric_limits<value_type>::max());
    }
  };
};

template <class Container>
class IdentityBackend {
 public:
  using DataSequence = Container;

  class Encoder {
    using value_type = typename DataSequence::value_type;

   public:
    template <std::output_iterator<value_type> IteratorType>
    static PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      *i++ = val;
    }

    static PROMPP_ALWAYS_INLINE void clear() noexcept {}

    static PROMPP_ALWAYS_INLINE bool empty() noexcept { return true; }

    template <std::output_iterator<value_type> IteratorType>
    static PROMPP_ALWAYS_INLINE void flush(IteratorType&) noexcept {}
  };

  class Decoder {
    using value_type = typename DataSequence::value_type;

   public:
    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE Decoder(IteratorType&, const IteratorSentinelType&, const Encoder&) noexcept {}

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE value_type decode(const IteratorType& begin, const IteratorSentinelType&, const Encoder&) const noexcept {
      return *begin;
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder&) noexcept {
      if (begin != end) {
        ++begin;
      }
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE bool is_finished(const IteratorType& begin, const IteratorSentinelType& end, const Encoder&) const noexcept {
      return begin == end;
    }
  };
};

template <template <class> class Backend, class Container>
class DeltaTransform {
 public:
  using DataSequence = Container;

  class Encoder : public Backend<DataSequence>::Encoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename Backend<DataSequence>::Encoder;

   public:
    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      assert(val >= last_);
      Base::encode(val - last_, i);
      last_ = val;
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept {
      Base::clear();
      last_ = 0;
    }

    PROMPP_ALWAYS_INLINE bool empty() noexcept { return last_ == 0 && Base::empty(); }
  };

  class Decoder : public Backend<DataSequence>::Decoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename Backend<DataSequence>::Decoder;

   public:
    using Base::Base;

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_ + Base::decode(begin, end, encoder);
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      last_ += Base::decode(begin, end, encoder);
      Base::next(begin, end, encoder);
    }
  };
};

template <template <class> class Backend, class Container>
class DeltaZigZagTransform {
 public:
  using DataSequence = Container;

  class Encoder : public Backend<DataSequence>::Encoder {
    using value_type = typename DataSequence::value_type;
    typedef typename std::make_signed<value_type>::type int_type;

    value_type last_ = 0;
    using Base = typename Backend<DataSequence>::Encoder;

   public:
    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      Base::encode(ZigZag::encode(std::bit_cast<int_type>(val) - std::bit_cast<int_type>(last_)), i);
      last_ = val;
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept {
      Base::clear();
      last_ = 0;
    }

    PROMPP_ALWAYS_INLINE bool empty() noexcept { return last_ == 0 && Base::empty(); }
  };

  class Decoder : public Backend<DataSequence>::Decoder {
    using value_type = typename DataSequence::value_type;

    value_type last_ = 0;
    using Base = typename Backend<DataSequence>::Decoder;

   public:
    using Base::Base;

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_ + ZigZag::decode(Base::decode(begin, end, encoder));
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      last_ += ZigZag::decode(Base::decode(begin, end, encoder));
      Base::next(begin, end, encoder);
    }
  };
};

template <template <class> class Backend, class Container>
class DeltaDeltaZigZagTransform {
 public:
  using DataSequence = Container;

  class Encoder : public Backend<DataSequence>::Encoder {
    using value_type = typename DataSequence::value_type;
    using int_type = std::make_signed_t<value_type>;

    value_type last_ = 0;
    int_type last_delta_ = 0;
    using Base = typename Backend<DataSequence>::Encoder;

   public:
    template <std::output_iterator<value_type> IteratorType>
    PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
      const int_type curr_delta = std::bit_cast<int_type>(val) - std::bit_cast<int_type>(last_);
      Base::encode(ZigZag::encode(curr_delta - last_delta_), i);
      last_delta_ = curr_delta;
      last_ = val;
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept {
      Base::clear();
      last_ = 0;
      last_delta_ = 0;
    }

    PROMPP_ALWAYS_INLINE bool empty() noexcept { return last_ == 0 && last_delta_ == 0 && Base::empty(); }
  };

  class Decoder : public Backend<DataSequence>::Decoder {
    using value_type = typename DataSequence::value_type;
    using int_type = std::make_signed_t<value_type>;

    value_type last_ = 0;
    int_type last_delta_ = 0;
    using Base = typename Backend<DataSequence>::Decoder;

   public:
    using Base::Base;

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE value_type decode(const IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) const noexcept {
      return last_ + last_delta_ + ZigZag::decode(Base::decode(begin, end, encoder));
    }

    template <std::input_iterator IteratorType, class IteratorSentinelType>
      requires std::is_same<typename std::iterator_traits<IteratorType>::value_type, value_type>::value && std::sentinel_for<IteratorSentinelType, IteratorType>
    PROMPP_ALWAYS_INLINE void next(IteratorType& begin, const IteratorSentinelType& end, const Encoder& encoder) noexcept {
      last_delta_ += ZigZag::decode(Base::decode(begin, end, encoder));
      last_ += last_delta_;
      Base::next(begin, end, encoder);
    }
  };
};

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
using RLE = RLEBackend<Container>;

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
using DeltaRLE = DeltaTransform<RLEBackend, Container>;

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
using DeltaZigZagRLE = DeltaZigZagTransform<RLEBackend, Container>;

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
using Delta = DeltaTransform<IdentityBackend, Container>;

template <class Container = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
using DeltaDeltaZigZag = DeltaDeltaZigZagTransform<IdentityBackend, Container>;

constexpr uint8_t kEncodingTypeTotalNumber = 5;

enum class ValueTypeSize : uint8_t { kBits32 = 0, kBits64 = 1 };

template <typename T>
constexpr ValueTypeSize get_value_type_size()
  requires std::is_unsigned_v<T> && (sizeof(T) >= 4)
{
  return sizeof(T) == 8 ? ValueTypeSize::kBits64 : ValueTypeSize::kBits32;
}

enum EncodingMethodID : uint8_t { RLE_ID = 0, DeltaRLE_ID = 1, DeltaZigZagRLE_ID = 2, Delta_ID = 3, DeltaDeltaZigZag_ID = 4 };

template <typename E>
struct id;

template <class DataSequence>
struct id<RLE<DataSequence>>
    : std::integral_constant<uint8_t, RLE_ID + kEncodingTypeTotalNumber * std::to_underlying(get_value_type_size<typename DataSequence::value_type>())> {};

template <class DataSequence>
struct id<DeltaRLE<DataSequence>>
    : std::integral_constant<uint8_t, DeltaRLE_ID + kEncodingTypeTotalNumber * std::to_underlying(get_value_type_size<typename DataSequence::value_type>())> {};

template <class DataSequence>
struct id<DeltaZigZagRLE<DataSequence>>
    : std::integral_constant<uint8_t,
                             DeltaZigZagRLE_ID + kEncodingTypeTotalNumber * std::to_underlying(get_value_type_size<typename DataSequence::value_type>())> {};

template <class DataSequence>
struct id<Delta<DataSequence>>
    : std::integral_constant<uint8_t, Delta_ID + kEncodingTypeTotalNumber * std::to_underlying(get_value_type_size<typename DataSequence::value_type>())> {};

template <class DataSequence>
struct id<DeltaDeltaZigZag<DataSequence>>
    : std::integral_constant<uint8_t,
                             DeltaDeltaZigZag_ID + kEncodingTypeTotalNumber * std::to_underlying(get_value_type_size<typename DataSequence::value_type>())> {};
}  // namespace Encoding

template <class E, class DataSequence = typename E::DataSequence>
class EncodedSequence {
  typename E::Encoder encoder_;

  DataSequence data_;

 public:
  using value_type = typename DataSequence::value_type;

  EncodedSequence() = default;
  EncodedSequence(const EncodedSequence&) = delete;
  EncodedSequence& operator=(const EncodedSequence&) = delete;

  PROMPP_ALWAYS_INLINE EncodedSequence(EncodedSequence&& o) noexcept : encoder_(std::move(o.encoder_)), data_(std::move(o.data_)) {}

  PROMPP_ALWAYS_INLINE EncodedSequence& operator=(EncodedSequence&& o) noexcept {
    encoder_ = std::move(o.encoder_);
    data_ = std::move(o.data_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE void push_back(value_type val) noexcept {
    std::back_insert_iterator<decltype(data_)> data_back_inserter{data_};
    encoder_.encode(val, data_back_inserter);
  }

  PROMPP_ALWAYS_INLINE void flush() noexcept {
    std::back_insert_iterator<decltype(data_)> data_back_inserter{data_};
    encoder_.flush(data_back_inserter);
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    encoder_.clear();
    data_.clear();
  }

  PROMPP_ALWAYS_INLINE const DataSequence& data() const noexcept { return data_; }

  class IteratorSentinel {};

  class Iterator {
    typename DataSequence::const_iterator begin_;
    typename DataSequence::sentinel end_;
    const typename E::Encoder* encoder_;
    typename E::Decoder decoder_;

   public:
    using iterator_category = std::input_iterator_tag;
    using value_type = typename DataSequence::value_type;
    using difference_type = std::ptrdiff_t;

    PROMPP_ALWAYS_INLINE Iterator(typename DataSequence::const_iterator begin, typename DataSequence::sentinel end, const typename E::Encoder* encoder) noexcept
        : begin_(begin), end_(end), encoder_(encoder), decoder_(begin_, end_, *encoder_) {}

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      decoder_.next(begin_, end_, *encoder_);
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      Iterator retval = *this;
      ++(*this);
      return retval;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return decoder_.is_finished(begin_, end_, *encoder_); }

    PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return decoder_.decode(begin_, end_, *encoder_); }
  };

  using iterator_type = Iterator;
  using const_iterator_type = Iterator;
  using sentinel = IteratorSentinel;

  PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(data_.begin(), data_.end(), &encoder_); }

  static PROMPP_ALWAYS_INLINE auto end() noexcept { return IteratorSentinel(); }

  PROMPP_ALWAYS_INLINE size_t save_size() noexcept {
    flush();

    // version is written and read by methods put() and get() and they write and read 1 byte
    return 1 + sizeof(Encoding::id<E>::value) + data_.save_size();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return data_.allocated_memory(); }

  template <OutputStream S>
  friend S& operator<<(S& out, EncodedSequence& seq) {
    seq.flush();

    auto original_exceptions = out.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
    out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

    // write version
    out.put(1);

    // write encoding id
    out.write(reinterpret_cast<const char*>(&Encoding::id<E>::value), sizeof(Encoding::id<E>::value));

    // write data
    out << seq.data_;

    return out;
  }

  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, EncodedSequence& seq) {
    assert(seq.data_.empty());
    assert(seq.encoder_.empty());
    auto sg1 = std::experimental::scope_fail([&]() { seq.clear(); });

    // read version
    uint8_t version = in.get();

    // return successfully, if stream is empty
    if (in.eof())
      return in;

    // check version
    if (version != 1) {
      throw BareBones::Exception(0xa506b0dd57836363, "Invalid EncodingSequence version %d got from input, only version 1 is supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg2 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read encoding id
    typename std::remove_const<decltype(Encoding::id<E>::value)>::type encoding_id;
    in.read(reinterpret_cast<char*>(&encoding_id), sizeof(encoding_id));
    if (encoding_id != Encoding::id<E>::value) {
      throw BareBones::Exception(0x1e1b301b2eb969ca, "Invalid encoder id %d while reading from input stream, expected id %d", encoding_id,
                                 Encoding::id<E>::value);
    }

    // read data
    in >> seq.data_;

    return in;
  }
};

template <class T>
struct IsTriviallyReallocatable<BareBones::EncodedSequence<BareBones::Encoding::DeltaRLE<T>>> : std::true_type {};
}  // namespace BareBones
