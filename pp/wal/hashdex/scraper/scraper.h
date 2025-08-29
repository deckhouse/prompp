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
          if (const auto error = MetricParser{parser_, metric_buffer_, metric_buffer_.add_metric(), labels_, default_timestamp}.parse();
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
    [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return scraper_.metric_buffer_.begin(scraper_.parser_.tokenizer().buffer()); }
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
  [[nodiscard]] PROMPP_LAMBDA_INLINE auto begin() const noexcept { return metric_buffer_.begin(parser_.tokenizer().buffer()); }
  [[nodiscard]] PROMPP_LAMBDA_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsWrapper metrics() const noexcept { return MetricsWrapper{*this}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataWrapper metadata() const noexcept { return MetadataWrapper{*this}; }

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
    uint32_t offset{};
    uint32_t offset_extra{};
    uint8_t bytes[];

    // explicit MarkedMetric(Primitives::Timestamp timestamp) : sample(timestamp, 0.0) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t occupied_size() const noexcept { return sizeof(*this) + sizeof(MarkedLabel) /*add bytes skip!!!*/; }
  };

  struct MarkedMetadata {
    MarkedString metric_name{};
    MarkedString text{};
    Prometheus::MetadataType type{};

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t occupied_size() const noexcept { return sizeof(*this); }
  };
#pragma pack(pop)

 public:
  class Metric {
   public:
    using MarkedItem = MarkedMetric;

    explicit Metric(std::string_view buffer, const MarkedMetric* item) : buffer_(buffer), item_(item) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetric* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetric* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

    template <class Timeseries>
    void read(Timeseries& timeseries) const {
      timeseries.label_set().reserve(item_->label_set.count);
      for (uint32_t i = 0; i < item_->label_set.count; ++i) {
        const auto& [name, value] = item_->label_set.labels[i];
        timeseries.label_set().append(name.view(buffer_), value.view(buffer_));
      }

      timeseries.samples().emplace_back(item_->sample);
    }

   private:
    std::string_view buffer_;
    const MarkedMetric* item_{};
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
        item_.set_item(reinterpret_cast<const MarkedItem*>(reinterpret_cast<const char*>(item_.item()) + item_.item()->occupied_size()));
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

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return buffer_.allocated_memory(); }

   protected:
    BareBones::Vector<char> buffer_;
    uint32_t items_count_{};
  };

  class MetricMarkupBuffer : public MarkupBuffer<Metric> {
   public:
    PROMPP_ALWAYS_INLINE MarkedMetric* add_metric() noexcept {
      ++this->items_count_;

      const auto offset = this->buffer_.size();
      this->buffer_.resize(offset + sizeof(MarkedMetric));
      return std::construct_at(reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset));
    }

    PROMPP_ALWAYS_INLINE void add_count(uint32_t count, MarkedMetric*& metric) noexcept {
      constexpr uint32_t kVarint1Byte = 0x80;        // 2^7
      constexpr uint32_t kVarint2Byte = 0x4000;      // 2^14
      constexpr uint32_t kVarint3Byte = 0x200000;    // 2^21
      constexpr uint32_t kVarint4Byte = 0x10000000;  // 2^28
      constexpr uint8_t kContinueBit = 0x80;
      constexpr uint8_t kValueMask = 0x7F;

      const auto offset = reinterpret_cast<const char*>(metric) - this->buffer_.data();

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

      this->buffer_.push_back(tmp.data(), out);

      metric = reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset);
    }

    PROMPP_ALWAYS_INLINE void add_label(const MarkedLabel& label, MarkedMetric*& metric) noexcept {
      if (label.value.is_empty()) [[unlikely]] {
        return;
      }

      auto name = label.name;
      if (name.is_reserved_name()) [[unlikely]] {
        name.offset = 0;
        name.length = 0;
      }

      const uint8_t sz0 = encode_size(name.offset);
      const uint8_t sz1 = encode_size(name.length);
      const uint8_t sz2 = encode_size(label.value.offset);
      const uint8_t sz3 = encode_size(label.value.length);

      const uint8_t layout = (sz0) | (sz1 << 2) | (sz2 << 4) | (sz3 << 6);

      std::array<char, 17> tmp{};
      char* out = tmp.data();

      *out++ = static_cast<char>(layout);

      *reinterpret_cast<uint32_t*>(out) = name.offset;
      out += sz0 + 1;

      *reinterpret_cast<uint32_t*>(out) = name.length;
      out += sz1 + 1;

      *reinterpret_cast<uint32_t*>(out) = label.value.offset;
      out += sz2 + 1;

      *reinterpret_cast<uint32_t*>(out) = label.value.length;
      out += sz3 + 1;

      const auto offset = reinterpret_cast<const char*>(metric) - this->buffer_.data();

      this->buffer_.push_back(tmp.data(), out);

      metric = reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset);
    }

    PROMPP_ALWAYS_INLINE void add_sample(const MarkedSample& sample, MarkedMetric*& metric) noexcept {
      const auto offset = reinterpret_cast<const char*>(metric) - this->buffer_.data();

      add_sample_internal(sample);

      metric = reinterpret_cast<MarkedMetric*>(this->buffer_.data() + offset);
    }

    PROMPP_ALWAYS_INLINE void add_sample_internal(const MarkedSample& sample) noexcept {
      uint8_t marker = sample.has_ts ? 0b10000000 : 0;
      const double val = sample.sample.value();

      auto flush = [&](uint8_t m) PROMPP_LAMBDA_INLINE {
        this->buffer_.push_back(marker | m);
        if (sample.has_ts) {
          append(sample.sample.timestamp());
        }
      };

      if (std::isnan(val)) [[unlikely]] {
        flush(0b00000100);  // staleNaN
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
    template <class T>
    PROMPP_ALWAYS_INLINE void append(const T val) noexcept {
      auto p = reinterpret_cast<const char*>(&val);
      this->buffer_.push_back(p, p + sizeof(T));
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
                 MarkedMetric* metric,
                 BareBones::Vector<MarkedLabel>& labels,
                 Primitives::Timestamp timestamp)
        : parser_(parser), markup_buffer_(markup_buffer), metric_(metric), labels_(labels), sample_{.sample = {timestamp, 0.0}} {}

    [[nodiscard]] Error parse() noexcept {
      labels_.clear();

      bool have_metric_name = false;
      auto& tokenizer = parser_.tokenizer();

      metric_->offset = tokenizer.token_str().data() - tokenizer.buffer().data();

      if (tokenizer.token() == Token::kMetricName) [[likely]] {
        // markup_buffer_.add_label(MarkedLabel{.value = MarkedString::create(tokenizer.token_str(), tokenizer.buffer())}, metric_);

        auto string = MarkedString::create(tokenizer.token_str(), tokenizer.buffer());
        string.offset -= metric_->offset;
        auto label = MarkedLabel{.value = string};
        labels_.push_back(label);

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

      // metric_->calculate_hash(tokenizer.buffer());
      // sort
      std::sort(labels_.begin(), labels_.end(), [buffer = tokenizer.buffer()](const MarkedLabel& a, const MarkedLabel& b) PROMPP_LAMBDA_INLINE {
        return a.name.view(buffer) < b.name.view(buffer);
      });

      // hash
      {
        BareBones::XXHash hash;
        for (const auto& label : labels_) {
          hash.extend(label.name.view(tokenizer.buffer()), label.value.view(tokenizer.buffer()));
        }
        metric_->hash = hash.hash();
      }

      // encode count
      {
        markup_buffer_.add_count(labels_.size(), metric_);
      }

      // encode labels
      for (const auto& label : labels_) {
        markup_buffer_.add_label(label, metric_);
      }

      return parse_metric_suffix();
    }

   private:
    Parser& parser_;
    MetricMarkupBuffer& markup_buffer_;
    MarkedMetric* metric_;
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

          // markup_buffer_.add_label(label, metric_);
          labels_.push_back(label);

          tokenizer.next();
        } else {
          if (!have_metric_name) [[unlikely]] {
            // markup_buffer_.add_label(MarkedLabel{.value = label.name}, metric_);
            labels_.push_back(label);

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
        label_name.offset -= metric_->offset;
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
      string.offset -= metric_->offset;
      return Error::kNoError;
    }

    [[nodiscard]] Error parse_metric_suffix() noexcept {
      if (!parser_.is_value_token()) [[unlikely]] {
        return Error::kUnexpectedToken;
      }

      if (const auto error = parse_sample(); error != Error::kNoError) {
        return error;
      }

      markup_buffer_.add_sample(sample_, metric_);

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
};

using PrometheusScraper = Scraper<PrometheusParser>;
using OpenMetricsScraper = Scraper<OpenMetricsParser>;

static_assert(Prometheus::hashdex::HashdexInterface<PrometheusScraper>);
static_assert(Prometheus::hashdex::HashdexInterface<OpenMetricsScraper>);

}  // namespace PromPP::WAL::hashdex::scraper
