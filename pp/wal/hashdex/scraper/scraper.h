#pragma once

#include <simdutf/simdutf.h>

#include <iostream>
#include <span>

#include "bare_bones/algorithm.h"
#include "bare_bones/vector.h"
#include "bare_bones/xxhash.h"
#include "parser.h"
#include "prometheus/hashdex.h"
#include "prometheus/metric.h"
#include "prometheus/textparse/escape.h"
#include "prometheus/value.h"

#include "primitives/sample.h"

namespace PromPP::WAL::hashdex::scraper {

template <ParserInterface Parser>
class Scraper {
 public:
  [[nodiscard]] Error parse(std::span<char> buffer, Primitives::Timestamp default_timestamp) {
    labels_.reserve(255);
    default_timestamp_ = default_timestamp;

    auto& tokenizer = parser_.tokenizer();
    tokenizer.tokenize({buffer.data(), buffer.data() + buffer.size()});

    while (true) {
      switch (tokenizer.next()) {
        case Token::kEOF:
        case Token::kEOFWord: {
          return parser_.validate_parse_result();
        }

        case Token::kWhitespace:
        case Token::kLinebreak:
        case Token::kComment: {
          break;
        }

        case Token::kHelp:
        case Token::kUnit:
        case Token::kType: {
          if (const auto error = parse_metadata(); error != Error::kNoError) [[unlikely]] {
            return error;
          }
          break;
        }

        case Token::kMetricName:
        case Token::kBraceOpen: {
          if (const auto error = MetricParser{parser_, metric_buffer_, labels_, static_cast<uint32_t>(tokenizer.token_str().data() - tokenizer.buffer().data()),
                                              default_timestamp}
                                     .parse();
              error != Error::kNoError) [[unlikely]] {
            metric_buffer_.remove_item();
            return error;
          }

          if (tokenizer.token() == Token::kExemplar) {
            tokenizer.consume_comment();
          }

          break;
        }

        default: {
          return Error::kUnexpectedToken;
        }
      }
    }
  }

  class MetricsWrapper {
   public:
    explicit MetricsWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return scraper_.metric_buffer_.items_count(); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept {
      return scraper_.metric_buffer_.begin(scraper_.parser_.tokenizer().buffer(), scraper_.default_timestamp());
    }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  class MetadataWrapper {
   public:
    explicit MetadataWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return scraper_.metadata_buffer_.items_count(); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metadata_buffer_.begin(scraper_.parser_.tokenizer().buffer()); }
    [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetadataMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint32_t size() const noexcept { return metric_buffer_.items_count(); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return metric_buffer_.begin(parser_.tokenizer().buffer(), default_timestamp_); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsWrapper metrics() const noexcept { return MetricsWrapper{*this}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataWrapper metadata() const noexcept { return MetadataWrapper{*this}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Timestamp default_timestamp() const noexcept { return default_timestamp_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return metric_buffer_.allocated_memory() + metadata_buffer_.allocated_memory();
  }

 private:
  using Token = Prometheus::textparse::Token;

#pragma pack(push, 1)
  struct MarkedString {
    uint32_t offset{std::numeric_limits<uint32_t>::max()};
    uint32_t length{std::numeric_limits<uint32_t>::max()};

    [[nodiscard]] PROMPP_ALWAYS_INLINE static MarkedString create(const std::string_view& value, const std::string_view& buffer) noexcept {
      return {
          .offset = static_cast<uint32_t>(value.data() - buffer.data()),
          .length = static_cast<uint32_t>(value.size()),
      };
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_reserved_name() const noexcept {
      return offset == std::numeric_limits<uint32_t>::max() && length == std::numeric_limits<uint32_t>::max();
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return length == 0; }

    [[nodiscard]] std::string_view view(const std::string_view& buffer) const noexcept {
      if (is_reserved_name()) [[unlikely]] {
        return Prometheus::kMetricLabelName;
      }

      return buffer.substr(offset, length);
    }
  };

  struct MarkedLabel {
    MarkedString name{};
    MarkedString value;
  };

  struct MarkedSample {
    Primitives::Sample sample{};
    bool has_ts{};
  };

  struct MarkedMetric {
    uint64_t hash{};
    uint32_t base_offset{};
    uint32_t data_offset{};
  };

  struct MarkedMetadata {
    MarkedString metric_name{};
    MarkedString text{};
    Prometheus::MetadataType type{};
  };
#pragma pack(pop)

 public:
  class Metric {
   public:
    using MarkedItem = MarkedMetric;

    Metric(std::string_view buffer, const BareBones::Vector<char>& bytes_buffer, const MarkedMetric* item, Primitives::Timestamp default_timestamp)
        : buffer_(buffer), bytes_buffer_(bytes_buffer), item_(item), default_timestamp_(default_timestamp) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetric* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetric* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

    template <class Timeseries>
    void read(Timeseries& ts) const {
      const char* ptr = bytes_buffer_.data() + item_->data_offset;
      const char* base = buffer_.data() + item_->base_offset;

      // decode label count
      uint32_t labels_count = decode_varint(ptr);
      ts.label_set().reserve(labels_count);

      // decode label
      for (uint32_t i = 0; i < labels_count; ++i) {
        const uint8_t layout = static_cast<uint8_t>(*ptr++);
        const uint8_t sz0 = (layout >> 0) & 0x3;
        const uint8_t sz1 = (layout >> 2) & 0x3;
        const uint8_t sz2 = (layout >> 4) & 0x3;
        const uint8_t sz3 = (layout >> 6) & 0x3;

        auto read_val = [&](uint8_t sz) PROMPP_LAMBDA_INLINE -> uint32_t {
          uint32_t v = 0;
          memcpy(&v, ptr, sz + 1);
          ptr += sz + 1;
          return v;
        };

        uint32_t name_off = read_val(sz0);
        uint32_t name_len = read_val(sz1);
        uint32_t value_off = read_val(sz2);
        uint32_t value_len = read_val(sz3);

        if (name_off == 0 && name_len == 0) [[unlikely]] {
          ts.label_set().append(Prometheus::kMetricLabelName, std::string_view(base + value_off, value_len));
        } else {
          ts.label_set().append(std::string_view(base + name_off, name_len), std::string_view(base + value_off, value_len));
        }
      }

      // decode sample
      uint8_t marker = static_cast<uint8_t>(*ptr++);
      bool has_ts = (marker & 0b10000000) != 0;
      uint8_t type = marker & 0b01111111;

      Primitives::Sample sample{};
      sample.timestamp() = default_timestamp_;

      if (has_ts) [[unlikely]] {
        int64_t ts_val;
        memcpy(&ts_val, ptr, sizeof(ts_val));
        ptr += sizeof(ts_val);
        sample.timestamp() = ts_val;
      }

      double val;

      switch (type) {
        case 0b00000011: {  // uint32
          uint32_t tmp;
          memcpy(&tmp, ptr, sizeof(tmp));
          ptr += sizeof(tmp);
          val = static_cast<double>(tmp);
          break;
        }
        case 0b00000000:  // zero
          val = 0.0;
          break;
        case 0b00000001: {  // uint8
          uint8_t tmp = static_cast<uint8_t>(*ptr++);
          val = static_cast<double>(tmp);
          break;
        }
        case 0b00000010: {  // uint16
          uint16_t tmp;
          memcpy(&tmp, ptr, sizeof(tmp));
          ptr += sizeof(tmp);
          val = static_cast<double>(tmp);
          break;
        }
        case 0b00000100:  // NaN
          val = Prometheus::kNormalNan;
          break;
        case 0b00001000: {  // float32
          float tmp;
          memcpy(&tmp, ptr, sizeof(tmp));
          ptr += sizeof(tmp);
          val = static_cast<double>(tmp);
          break;
        }
        case 0b00001001: {  // double
          double tmp;
          memcpy(&tmp, ptr, sizeof(tmp));
          ptr += sizeof(tmp);
          val = tmp;
          break;
        }
        default:
          val = Prometheus::kStaleNan;
          break;
      }

      sample.value() = val;
      ts.samples().emplace_back(sample);
    }

   private:
    std::string_view buffer_;
    const BareBones::Vector<char>& bytes_buffer_;
    const MarkedMetric* item_{};
    Primitives::Timestamp default_timestamp_;

    static uint32_t decode_varint(const char*& ptr) noexcept {
      uint32_t b0 = static_cast<uint8_t>(*ptr++);
      if ((b0 & 0x80) == 0) [[likely]] {
        return b0;
      }

      uint32_t b1 = static_cast<uint8_t>(*ptr++);
      uint32_t v = (b0 & 0x7F) | ((b1 & 0x7F) << 7);
      if ((b1 & 0x80) == 0) {
        return v;
      }

      uint32_t b2 = static_cast<uint8_t>(*ptr++);
      v |= (b2 & 0x7F) << 14;
      if ((b2 & 0x80) == 0) {
        return v;
      }

      uint32_t b3 = static_cast<uint8_t>(*ptr++);
      v |= (b3 & 0x7F) << 21;
      if ((b3 & 0x80) == 0) {
        return v;
      }

      uint32_t b4 = static_cast<uint8_t>(*ptr++);
      v |= (b4 & 0x0F) << 28;
      return v;
    }
  };

  class Metadata {
   public:
    using MarkedItem = MarkedMetadata;

    explicit Metadata(std::string_view buffer, const MarkedMetadata* item) : buffer_(buffer), item_(item) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetadata* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetadata* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Prometheus::MetadataType type() const noexcept { return item_->type; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view metric_name() const noexcept { return item_->metric_name.view(buffer_); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view text() const noexcept { return item_->text.view(buffer_); }

   private:
    std::string_view buffer_;
    const MarkedMetadata* item_{};
  };

 private:
  template <class Item>
  class MarkupBuffer {
   public:
    class IteratorSentinel {};

    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = Item;
      using difference_type = ptrdiff_t;
      using pointer = value_type*;
      using reference = value_type&;
      using MarkedItem = typename Item::MarkedItem;

      Iterator(std::string_view buffer, const MarkupBuffer* markup_buffer)
          : item_(buffer, reinterpret_cast<const MarkedItem*>(markup_buffer->buffer().data())), items_count_(markup_buffer->items_count()) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type& operator*() const noexcept { return item_; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type* operator->() const noexcept { return &item_; }

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        item_.set_item(reinterpret_cast<const MarkedItem*>(reinterpret_cast<const char*>(item_.item()) + sizeof(*item_.item())));
        --items_count_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        const auto it = *this;
        ++*this;
        return it;
      }

      PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return items_count_ == 0; }

     private:
      Item item_;
      uint32_t items_count_;
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<char>& buffer() const noexcept { return buffer_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t items_count() const noexcept { return items_count_; }

    PROMPP_ALWAYS_INLINE void remove_item() noexcept { --items_count_; }

    PROMPP_ALWAYS_INLINE void initialize(size_t reserve) noexcept {
      buffer_.clear();
      buffer_.reserve(reserve);
      items_count_ = 0;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer) const noexcept { return {buffer, this}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return buffer_.size(); }

   protected:
    BareBones::Vector<char> buffer_;
    uint32_t items_count_{};
  };

  class MetricMarkupBuffer {
   public:
    class IteratorSentinel {};

    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = Metric;
      using difference_type = ptrdiff_t;
      using pointer = value_type*;
      using reference = value_type&;

      Iterator(std::string_view buffer,
               const BareBones::Vector<char>& bytes_buffer,
               const MarkedMetric* ptr,
               uint32_t items_count,
               Primitives::Timestamp default_timestamp)
          : item_(buffer, bytes_buffer, ptr, default_timestamp), ptr_(ptr), buffer_(buffer), bytes_buffer_(bytes_buffer), items_count_(items_count) {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type& operator*() const noexcept { return item_; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE const value_type* operator->() const noexcept { return &item_; }

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        item_.set_item(++ptr_);
        --items_count_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        auto tmp = *this;
        ++(*this);
        return tmp;
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return items_count_ == 0; }

     private:
      Metric item_;
      const MarkedMetric* ptr_{};
      std::string_view buffer_;
      const BareBones::Vector<char>& bytes_buffer_;
      uint32_t items_count_{};
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer, Primitives::Timestamp default_timestamp) const noexcept {
      return {buffer, bytes_buffer_, metric_buffer_.data(), metric_buffer_.size(), default_timestamp};
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<MarkedMetric>& metric_buffer() const noexcept { return metric_buffer_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::Vector<char>& bytes_buffer() const noexcept { return bytes_buffer_; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t items_count() const noexcept { return metric_buffer_.size(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t bytes_count() const noexcept { return bytes_buffer_.size(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return metric_buffer_.size() * sizeof(MarkedMetric) + bytes_buffer_.size(); }

    PROMPP_ALWAYS_INLINE void remove_item() noexcept {
      bytes_buffer_.resize(metric_buffer_.back().data_offset);
      metric_buffer_.resize(metric_buffer_.size() - 1);
    }

    PROMPP_ALWAYS_INLINE void add_hash(uint64_t hash) noexcept { metric_buffer_.back().hash = hash; }
    PROMPP_ALWAYS_INLINE void add_metric(uint32_t global_offset) noexcept {
      metric_buffer_.push_back(MarkedMetric{.base_offset = global_offset, .data_offset = bytes_count()});
    }

    PROMPP_ALWAYS_INLINE void add_count(uint32_t count) noexcept {
      constexpr uint32_t kVarint1Byte = 0x80;        // 2^7
      constexpr uint32_t kVarint2Byte = 0x4000;      // 2^14
      constexpr uint32_t kVarint3Byte = 0x200000;    // 2^21
      constexpr uint32_t kVarint4Byte = 0x10000000;  // 2^28
      constexpr uint8_t kContinueBit = 0x80;
      constexpr uint8_t kValueMask = 0x7F;

      std::array<char, 5> tmp{};
      char* out = tmp.data();

      if (count < kVarint1Byte) {
        *out++ = static_cast<char>(count);
      } else if (count < kVarint2Byte) {
        *out++ = static_cast<char>((count & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(count >> 7);
      } else if (count < kVarint3Byte) {
        *out++ = static_cast<char>((count & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 7) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(count >> 14);
      } else if (count < kVarint4Byte) {
        *out++ = static_cast<char>((count & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 7) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 14) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(count >> 21);
      } else {
        *out++ = static_cast<char>((count & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 7) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 14) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(((count >> 21) & kValueMask) | kContinueBit);
        *out++ = static_cast<char>(count >> 28);
      }

      bytes_buffer_.push_back(tmp.data(), out);
    }

    PROMPP_ALWAYS_INLINE void add_label(MarkedLabel label) noexcept {
      const auto& metric = metric_buffer_.back();

      if (label.name.is_reserved_name()) [[unlikely]] {
        label.name.offset = 0;
        label.name.length = 0;
      } else {
        label.name.offset -= metric.base_offset;
      }
      label.value.offset -= metric.base_offset;

      const uint8_t sz0 = encode_size(label.name.offset);
      const uint8_t sz1 = encode_size(label.name.length);
      const uint8_t sz2 = encode_size(label.value.offset);
      const uint8_t sz3 = encode_size(label.value.length);

      const uint8_t layout = (sz0) | (sz1 << 2) | (sz2 << 4) | (sz3 << 6);

      std::array<char, 17> tmp{};
      char* out = tmp.data();

      *out++ = static_cast<char>(layout);

      *reinterpret_cast<uint32_t*>(out) = label.name.offset;
      out += sz0 + 1;

      *reinterpret_cast<uint32_t*>(out) = label.name.length;
      out += sz1 + 1;

      *reinterpret_cast<uint32_t*>(out) = label.value.offset;
      out += sz2 + 1;

      *reinterpret_cast<uint32_t*>(out) = label.value.length;
      out += sz3 + 1;

      bytes_buffer_.push_back(tmp.data(), out);
    }

    PROMPP_ALWAYS_INLINE void add_sample(const MarkedSample& sample) noexcept {
      uint8_t marker = sample.has_ts ? 0b10000000 : 0;
      const double val = sample.sample.value();

      auto flush = [&](uint8_t m) PROMPP_LAMBDA_INLINE {
        bytes_buffer_.push_back(marker | m);
        if (sample.has_ts) {
          append(sample.sample.timestamp());
        }
      };

      if (std::isnan(val)) [[unlikely]] {
        flush(0b00000100);  // NaN
        return;
      }

      if (val == 0.0) [[unlikely]] {
        flush(0b00000000);  // zero
        return;
      }

      if (std::trunc(val) == val && val > 0.0) [[likely]] {  // u integer
        auto ival = static_cast<int64_t>(val);
        if (ival <= std::numeric_limits<uint8_t>::max()) [[unlikely]] {
          flush(0b00000001);
          append(static_cast<uint8_t>(ival));
          return;
        }
        if (ival <= std::numeric_limits<uint16_t>::max()) [[unlikely]] {
          flush(0b00000010);
          append(static_cast<uint16_t>(ival));
          return;
        }
        if (ival <= std::numeric_limits<uint32_t>::max()) [[likely]] {
          flush(0b00000011);
          append(static_cast<uint32_t>(ival));
          return;
        }
      }

      float f = static_cast<float>(val);
      if (static_cast<double>(f) == val) [[unlikely]] {
        flush(0b00001000);  // float32
        append(f);
      } else {
        flush(0b00001001);  // double
        append(val);
      }
    }

   private:
    BareBones::Vector<MarkedMetric> metric_buffer_;
    BareBones::Vector<char> bytes_buffer_;

    template <class T>
    PROMPP_ALWAYS_INLINE void append(const T val) noexcept {
      auto p = reinterpret_cast<const char*>(&val);
      bytes_buffer_.push_back(p, p + sizeof(T));
    }

    static uint8_t encode_size(uint32_t v) noexcept {
      if (v <= 0xFF)
        return 0;
      if (v <= 0xFFFF)
        return 1;
      if (v <= 0xFFFFFF)
        return 2;
      return 3;
    }
  };

  class MetadataMarkupBuffer : public MarkupBuffer<Metadata> {
   public:
    PROMPP_ALWAYS_INLINE void add(MarkedString metric_name, MarkedString text, Prometheus::MetadataType type) noexcept {
      ++this->items_count_;

      const auto offset = this->buffer_.size();
      this->buffer_.resize(offset + sizeof(MarkedMetadata));
      new (reinterpret_cast<MarkedMetadata*>(this->buffer_.data() + offset)) MarkedMetadata{
          .metric_name = metric_name,
          .text = text,
          .type = type,
      };
    }
  };

  class MetricParser {
   public:
    MetricParser(Parser& parser,
                 MetricMarkupBuffer& markup_buffer,
                 BareBones::Vector<MarkedLabel>& labels,
                 uint32_t global_offset,
                 Primitives::Timestamp timestamp)
        : parser_(parser), markup_buffer_(markup_buffer), labels_(labels), sample_{.sample = {timestamp, 0.0}} {
      markup_buffer_.add_metric(global_offset);
    }

    [[nodiscard]] Error parse() noexcept {
      labels_.clear();

      bool have_metric_name = false;
      auto& tokenizer = parser_.tokenizer();

      if (tokenizer.token() == Token::kMetricName) [[likely]] {
        labels_.push_back(MarkedLabel{.value = MarkedString::create(tokenizer.token_str(), tokenizer.buffer())});

        have_metric_name = true;
        tokenizer.next_non_whitespace();
      } else if (tokenizer.token() == Token::kWhitespace) [[likely]] {
        tokenizer.next();
      }

      if (tokenizer.token() == Token::kBraceOpen) [[likely]] {
        if (const auto error = tokenize_label_set(have_metric_name); error != Error::kNoError) {
          return error;
        }

        tokenizer.next_non_whitespace();
      } else if (!parser_.is_value_token()) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (!have_metric_name) [[unlikely]] {
        return Error::kNoMetricName;
      }

      // sort
      {
        const auto it = std::remove_if(labels_.begin(), labels_.end(), [](const MarkedLabel& label) { return label.value.is_empty(); });
        labels_.erase(it, labels_.end());
      }

      std::sort(labels_.begin(), labels_.end(), [buffer = tokenizer.buffer()](const MarkedLabel& a, const MarkedLabel& b) PROMPP_LAMBDA_INLINE {
        return a.name.view(buffer) < b.name.view(buffer);
      });

      // hash
      {
        BareBones::XXHash3 hash;
        for (const auto& label : labels_) {
          hash.extend(label.name.view(tokenizer.buffer()), label.value.view(tokenizer.buffer()));
        }
        markup_buffer_.add_hash(hash.hash());
      }

      // encode count
      { markup_buffer_.add_count(labels_.size()); }

      // encode labels
      for (const auto& label : labels_) {
        markup_buffer_.add_label(label);
      }

      return parse_metric_suffix();
    }

   private:
    Parser& parser_;
    MetricMarkupBuffer& markup_buffer_;
    BareBones::Vector<MarkedLabel>& labels_;
    MarkedSample sample_;

    [[nodiscard]] Error tokenize_label_set(bool& have_metric_name) noexcept {
      auto& tokenizer = parser_.tokenizer();
      tokenizer.next_non_whitespace();

      while (tokenizer.token() != Token::kBraceClose) {
        MarkedLabel label;
        if (const auto error = get_label_name(label.name); error != Error::kNoError) [[unlikely]] {
          return error;
        }

        if (tokenizer.next_non_whitespace() == Token::kEqual) [[likely]] {
          if (tokenizer.next_non_whitespace() != Token::kLabelValue) [[unlikely]] {
            return Error::kUnexpectedToken;
          }

          if (const auto error = get_quoted_value(label.value); error != Error::kNoError) [[unlikely]] {
            return error;
          }

          labels_.push_back(label);

          tokenizer.next();
        } else {
          if (!have_metric_name) [[unlikely]] {
            labels_.push_back(MarkedLabel{.value = label.name});

            have_metric_name = true;
          } else {
            return Error::kUnexpectedToken;
          }
        }

        if (tokenizer.token() != Token::kComma && tokenizer.token() != Token::kWhitespace) {
          break;
        }

        tokenizer.next_non_whitespace();
      }

      return tokenizer.token() == Token::kBraceClose ? Error::kNoError : Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_label_name(MarkedString& label_name) const noexcept {
      auto& tokenizer = parser_.tokenizer();

      if (tokenizer.token() == Token::kLabelName) [[likely]] {
        label_name = MarkedString::create(tokenizer.token_str(), tokenizer.buffer());
        return Error::kNoError;
      }
      if (tokenizer.token() == Token::kQuotedString) {
        return get_quoted_value(label_name);
      }

      return Error::kUnexpectedToken;
    }

    [[nodiscard]] Error get_quoted_value(MarkedString& string) const noexcept {
      auto& tokenizer = parser_.tokenizer();

      auto value = tokenizer.token_str();
      Prometheus::textparse::unquote(value);

      auto copy_to = const_cast<char*>(value.data());
      Prometheus::textparse::unescape_label_value(value, [&copy_to](const std::string_view& piece_of_string) {
        if (copy_to != piece_of_string.data()) [[unlikely]] {
          memmove(copy_to, piece_of_string.data(), piece_of_string.size());
        }

        copy_to += piece_of_string.size();
      });
      value.remove_suffix(value.size() - (copy_to - value.data()));

      if (!simdutf::validate_utf8(value.data(), value.size())) [[unlikely]] {
        return Error::kInvalidUtf8;
      }

      string = MarkedString::create(value, tokenizer.buffer());
      return Error::kNoError;
    }

    [[nodiscard]] Error parse_metric_suffix() noexcept {
      if (!parser_.is_value_token()) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (const auto error = parse_sample(); error != Error::kNoError) {
        return error;
      }

      markup_buffer_.add_sample(sample_);

      return parser_.validate_parse_sample_result();
    }

    [[nodiscard]] Error parse_sample() noexcept {
      auto& tokenizer = parser_.tokenizer();

      if (!parse_numeric_value(tokenizer.token_str(), sample_.sample.value())) [[unlikely]] {
        return Error::kInvalidValue;
      }
      if (std::isnan(sample_.sample.value())) [[unlikely]] {
        sample_.sample.value() = Prometheus::kNormalNan;
      }

      tokenizer.next_non_whitespace();

      return parser_.parse_timestamp(sample_.sample.timestamp(), sample_.has_ts);
    }
  };

  [[nodiscard]] Error parse_metadata() {
    static constexpr auto get_metadata_type = [](Token token) PROMPP_LAMBDA_INLINE {
      if (token == Token::kHelp) {
        return Prometheus::MetadataType::kHelp;
      }
      if (token == Token::kType) {
        return Prometheus::MetadataType::kType;
      }

      return Prometheus::MetadataType::kUnit;
    };

    auto& tokenizer = parser_.tokenizer();
    const auto type = tokenizer.token();

    if (tokenizer.next_non_whitespace() != Token::kMetricName) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    auto metric_name = tokenizer.token_str();
    Prometheus::textparse::unquote(metric_name);

    if (tokenizer.next_non_whitespace() != Token::kText) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    const auto text = tokenizer.token_str();
    if (const auto token = tokenizer.next_non_whitespace(); !BareBones::is_in(token, Token::kLinebreak, Token::kEOF)) [[unlikely]] {
      return Error::kUnexpectedToken;
    }

    if (type == Token::kHelp && !simdutf::validate_utf8(text.data(), text.size())) [[unlikely]] {
      return Error::kInvalidUtf8;
    }

    const auto buffer = tokenizer.buffer();
    metadata_buffer_.add(MarkedString::create(metric_name, buffer), MarkedString::create(text, buffer), get_metadata_type(type));
    return Error::kNoError;
  }

  Parser parser_;
  MetricMarkupBuffer metric_buffer_;
  MetadataMarkupBuffer metadata_buffer_;
  BareBones::Vector<MarkedLabel> labels_;
  Primitives::Timestamp default_timestamp_{};
};

using PrometheusScraper = Scraper<PrometheusParser>;
using OpenMetricsScraper = Scraper<OpenMetricsParser>;

static_assert(Prometheus::hashdex::HashdexInterface<PrometheusScraper>);
static_assert(Prometheus::hashdex::HashdexInterface<OpenMetricsScraper>);

}  // namespace PromPP::WAL::hashdex::scraper
