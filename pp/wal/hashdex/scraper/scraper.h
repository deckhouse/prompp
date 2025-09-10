#pragma once

#include <simdutf/simdutf.h>

#include <span>

#include "bare_bones/algorithm.h"
#include "bare_bones/bit.h"
#include "bare_bones/vector.h"
#include "bare_bones/xxhash.h"
#include "encoding.h"
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
    metric_buffer_.initialize(buffer.size() / 4);
    metadata_buffer_.initialize(buffer.size() / 128);
    labels_.reserve(255);

    default_timestamp_ = default_timestamp;

    auto& tokenizer = parser_.tokenizer();
    tokenizer.tokenize({buffer.data(), buffer.data() + buffer.size()});

    while (true) {
      switch (tokenizer.next()) {
        case Token::kEOF:
        case Token::kEOFWord: {
          metric_buffer_.add_padding();
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
          if (const auto error = parse_metric(); error != Error::kNoError) [[unlikely]] {
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

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return scraper_.metric_buffer_.items_count(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept {
      return scraper_.metric_buffer_.begin(scraper_.parser_.tokenizer().buffer(), scraper_.default_timestamp());
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  class MetadataWrapper {
   public:
    explicit MetadataWrapper(const Scraper& scraper) : scraper_(scraper) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return scraper_.metadata_buffer_.items_count(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return scraper_.metadata_buffer_.begin(scraper_.parser_.tokenizer().buffer()); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static auto end() noexcept { return MetadataMarkupBuffer::end(); }

   private:
    const Scraper& scraper_;
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return metric_buffer_.items_count(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return metric_buffer_.begin(parser_.tokenizer().buffer(), default_timestamp_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static auto end() noexcept { return MetricMarkupBuffer::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE MetricsWrapper metrics() const noexcept { return MetricsWrapper{*this}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MetadataWrapper metadata() const noexcept { return MetadataWrapper{*this}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Timestamp default_timestamp() const noexcept { return default_timestamp_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return metric_buffer_.allocated_memory() + metadata_buffer_.allocated_memory() + labels_.allocated_memory();
  }

 private:
  using Token = Prometheus::textparse::Token;

#pragma pack(push, 1)
  struct MarkedString {
    uint32_t offset = 0;
    uint32_t length = 0;

    [[nodiscard]] PROMPP_ALWAYS_INLINE static MarkedString create(std::string_view value, std::string_view buffer) noexcept {
      return {
          .offset = static_cast<uint32_t>(value.data() - buffer.data()),
          .length = static_cast<uint32_t>(value.size()),
      };
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_reserved_name() const noexcept { return offset == 0 && length == 0; }

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
    uint64_t hash;
    uint32_t base_offset;
    uint32_t data_offset;
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
    using MarkedT = MarkedMetric;

    struct Context {
      std::string_view buffer;
      const BareBones::Vector<char>& bytes_buffer;
      Primitives::Timestamp default_timestamp{};
    };

    Metric(const Context& ctx, const MarkedMetric* item)
        : buffer_(ctx.buffer), bytes_buffer_(ctx.bytes_buffer), item_(item), default_timestamp_(ctx.default_timestamp) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetric* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetric* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

    template <class Timeseries>
    void read(Timeseries& ts) const {
      const char* ptr = reinterpret_cast<const char*>(bytes_buffer_.data() + item_->data_offset);

      uint32_t labels_count{};
      encoding::LayoutMarker layout{};

      ptr = decode_count(ptr, layout, labels_count);
      ts.label_set().resize(labels_count);

      auto label_iter = ts.label_set().begin();
      const char* base = buffer_.data() + item_->base_offset;
      for (uint32_t i = 0; i < labels_count; ++i) {
        const auto [next_ptr, name_off, name_len, value_off, value_len] = encoding::LabelCodec::decode(ptr);
        ptr = next_ptr;

        if (name_len == 0 && name_off == 0) [[unlikely]] {
          std::construct_at(label_iter++, Prometheus::kMetricLabelName, std::string_view(base + value_off, value_len));
        } else {
          std::construct_at(label_iter++, std::string_view(base + name_off, name_len), std::string_view(base + value_off, value_len));
        }
      }

      auto [p, sample] = encoding::SampleCodec::decode(ptr, layout, default_timestamp_);

      ts.samples().emplace_back(sample);
    }

   private:
    std::string_view buffer_;
    const BareBones::Vector<char>& bytes_buffer_;
    const MarkedMetric* item_{};
    Primitives::Timestamp default_timestamp_;

    PROMPP_ALWAYS_INLINE static const char* decode_count(const char* ptr, encoding::LayoutMarker& layout, uint32_t& labels_count) noexcept {
      uint64_t chunk;
      std::memcpy(&chunk, ptr, sizeof(chunk));

      layout = encoding::LayoutMarker{.raw = static_cast<uint8_t>(chunk)};

      chunk >>= 8;
      const uint64_t mask = (1ULL << BareBones::Bit::to_bits(layout.count_size_in_bytes())) - 1;

      labels_count = static_cast<uint32_t>(chunk & mask);

      return ptr + 1 + layout.count_size_in_bytes();
    }
  };

  class Metadata {
   public:
    using MarkedT = MarkedMetadata;

    struct Context {
      std::string_view buffer;
    };

    Metadata(const Context& ctx, const MarkedMetadata* item) : buffer_(ctx.buffer), item_(item) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetadata* item() const noexcept { return item_; }
    PROMPP_ALWAYS_INLINE void set_item(const MarkedMetadata* item) noexcept { item_ = item; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Prometheus::MetadataType type() const noexcept { return item_->type; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view metric_name() const noexcept { return item_->metric_name.view(buffer_); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view text() const noexcept { return item_->text.view(buffer_); }

   private:
    std::string_view buffer_;
    const MarkedMetadata* item_{};
  };

  template <typename T>
  class MarkupBuffer {
   public:
    using MarkedT = typename T::MarkedT;
    using Context = typename T::Context;

    class IteratorSentinel {};

    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = T;
      using difference_type = ptrdiff_t;
      using pointer = value_type*;
      using reference = value_type&;

      Iterator(const Context& ctx, const MarkedT* ptr, uint32_t items_count) : item_(ctx, ptr), ptr_(ptr), items_count_(items_count), ctx_(ctx) {}

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

      PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return items_count_ == 0; }

     private:
      T item_;
      const MarkedT* ptr_;
      uint32_t items_count_;
      Context ctx_;
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t items_count() const noexcept { return buffer_.size(); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return buffer_.allocated_memory(); }

   protected:
    BareBones::Vector<MarkedT> buffer_;
  };

  class MetricMarkupBuffer : public MarkupBuffer<Metric> {
   public:
    using Base = MarkupBuffer<Metric>;
    using Iterator = typename Base::Iterator;
    using IteratorSentinel = typename Base::IteratorSentinel;

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer, Primitives::Timestamp default_ts) const noexcept {
      return {typename Base::Context{buffer, bytes_buffer_, default_ts}, this->buffer_.data(), this->items_count()};
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t bytes_count() const noexcept { return bytes_count_; }
    void bytes_enlarge(uint32_t new_size) noexcept {
      assert(new_size > bytes_count_);
      if (new_size > bytes_buffer_.size()) [[likely]] {
        bytes_buffer_.resize(new_size);
      }
      bytes_count_ = new_size;
    }

    void bytes_shrink(uint32_t new_size) noexcept { bytes_count_ = new_size; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return this->buffer_.allocated_memory() + bytes_buffer_.allocated_memory(); }

    PROMPP_ALWAYS_INLINE void initialize(size_t reserve_bytes) noexcept {
      this->buffer_.clear();

      const size_t bytes_buffer_reserve = (reserve_bytes / 3) * 2;
      const size_t items_buffer_reserve = (bytes_buffer_reserve / 3) / sizeof(MarkedMetric);

      this->buffer_.reserve(items_buffer_reserve);
      bytes_buffer_.reserve(bytes_buffer_reserve);
      bytes_count_ = 0;
    }

    PROMPP_ALWAYS_INLINE void add_hash(uint64_t hash) noexcept { this->buffer_.back().hash = hash; }

    PROMPP_ALWAYS_INLINE void add_metric(uint32_t global_offset) noexcept {
      this->buffer_.push_back(MarkedMetric{.hash = {}, .base_offset = global_offset, .data_offset = bytes_count()});
    }

    void add_layout(uint8_t layout) noexcept {
      const uint32_t offset = bytes_count();
      *(bytes_buffer_.data() + offset) = layout;
      bytes_shrink(offset + 1);
    }

    void add_count(uint32_t count) noexcept {
      const uint32_t bytes_written = (std::bit_width(count) + 7) / 8;
      const uint32_t offset = bytes_count();
      std::memcpy(bytes_buffer_.data() + offset, &count, sizeof(count));
      bytes_shrink(offset + bytes_written);
    }

    void add_label(MarkedLabel label) noexcept {
      const auto end =
          encoding::LabelCodec::encode(bytes_buffer_.data() + bytes_count(), label.name.offset, label.name.length, label.value.offset, label.value.length);
      bytes_shrink(end - bytes_buffer_.data());
    }

    void add_sample(encoding::LayoutMarker layout, const Primitives::Sample& sample) noexcept {
      using encoding::SampleValueType;

      char* data_ptr = bytes_buffer_.data();
      const char* end = encoding::SampleCodec::encode(data_ptr + bytes_count(), layout, sample);

      bytes_shrink(end - data_ptr);
    }

    void add_padding() noexcept {
      constexpr size_t kPaddingSizeBytes = 16;
      bytes_enlarge(bytes_count() + kPaddingSizeBytes);
    }

   private:
    BareBones::Vector<char> bytes_buffer_{};
    uint32_t bytes_count_ = 0;
  };

  class MetadataMarkupBuffer : public MarkupBuffer<Metadata> {
   public:
    using Base = MarkupBuffer<Metadata>;
    using Iterator = typename Base::Iterator;
    using IteratorSentinel = typename Base::IteratorSentinel;

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin(std::string_view buffer) const noexcept {
      return {typename Base::Context{buffer}, this->buffer_.data(), this->items_count()};
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    PROMPP_ALWAYS_INLINE void initialize(size_t reserve_bytes) noexcept {
      this->buffer_.clear();
      const size_t items_buffer_reserve = reserve_bytes / sizeof(MarkedMetric);
      this->buffer_.reserve(items_buffer_reserve);
    }

    PROMPP_ALWAYS_INLINE void add(MarkedString metric_name, MarkedString text, Prometheus::MetadataType type) noexcept {
      this->buffer_.emplace_back(metric_name, text, type);
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

  [[nodiscard]] Error parse_metric() {
    labels_.clear();

    marked_sample_ = {};
    marked_sample_.sample.timestamp() = default_timestamp_;

    bool have_metric_name = false;
    auto& tokenizer = parser_.tokenizer();

    const uint32_t metric_offset = tokenizer.token_str().data() - tokenizer.buffer().data();

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

    const auto error = parse_metric_suffix();

    if (error == Error::kNoError) [[likely]] {
      encode_metric_data(metric_offset);
    }

    return error;
  }

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

    return parser_.validate_parse_sample_result();
  }

  [[nodiscard]] Error parse_sample() noexcept {
    auto& tokenizer = parser_.tokenizer();

    if (!parse_numeric_value(tokenizer.token_str(), marked_sample_.sample.value())) [[unlikely]] {
      return Error::kInvalidValue;
    }
    if (std::isnan(marked_sample_.sample.value())) [[unlikely]] {
      marked_sample_.sample.value() = Prometheus::kNormalNan;
    }

    tokenizer.next_non_whitespace();

    return parser_.parse_timestamp(marked_sample_.sample.timestamp(), marked_sample_.has_ts);
  }

  void encode_metric_data(uint32_t metric_offset) noexcept {
    metric_buffer_.add_metric(metric_offset);

    const uint32_t bytes_offset = metric_buffer_.bytes_count();
    metric_buffer_.bytes_enlarge(bytes_offset + calculate_metric_prealloc_size(labels_.size()));
    metric_buffer_.bytes_shrink(bytes_offset);

    const encoding::LayoutMarker layout =
        encoding::LayoutMarker::make(marked_sample_.has_ts, labels_.size(), encoding::value_type(marked_sample_.sample.value()));
    metric_buffer_.add_layout(layout.raw);

    process_labels_buffer(metric_offset);
    metric_buffer_.add_sample(layout, marked_sample_.sample);
  }

  void process_labels_buffer(uint32_t offset) noexcept {
    sort_and_filter_labels();
    append_labels_hash();

    metric_buffer_.add_count(labels_.size());

    for (auto& label : labels_) {
      if (!label.name.is_reserved_name()) [[likely]] {
        label.name.offset -= offset;
      }
      label.value.offset -= offset;

      metric_buffer_.add_label(label);
    }
  }

  void sort_and_filter_labels() noexcept {
    const auto it = std::remove_if(labels_.begin(), labels_.end(), [](const MarkedLabel& label) PROMPP_LAMBDA_INLINE { return label.value.is_empty(); });
    labels_.erase(it, labels_.end());

    std::sort(labels_.begin(), labels_.end(), [buffer = parser_.tokenizer().buffer()](const MarkedLabel& a, const MarkedLabel& b) PROMPP_LAMBDA_INLINE {
      return a.name.view(buffer) < b.name.view(buffer);
    });
  }

  void append_labels_hash() noexcept {
    const auto& tokenizer = parser_.tokenizer();
    BareBones::XXHash hash;
    for (const auto& label : labels_) {
      hash.extend(label.name.view(tokenizer.buffer()), label.value.view(tokenizer.buffer()));
    }
    metric_buffer_.add_hash(hash.hash());
  }

  static PROMPP_ALWAYS_INLINE uint32_t calculate_metric_prealloc_size(uint32_t labels_count) noexcept {
    constexpr uint32_t kCountVarintBytes = 8;
    constexpr uint32_t kLabelBytes = 17;
    constexpr uint32_t kSampleSize = 16;

    return kCountVarintBytes + labels_count * kLabelBytes + kSampleSize;
  }

  Parser parser_;
  MetricMarkupBuffer metric_buffer_;
  MetadataMarkupBuffer metadata_buffer_;
  BareBones::Vector<MarkedLabel> labels_;
  Primitives::Timestamp default_timestamp_{};
  MarkedSample marked_sample_{};
};

using PrometheusScraper = Scraper<PrometheusParser>;
using OpenMetricsScraper = Scraper<OpenMetricsParser>;

static_assert(Prometheus::hashdex::HashdexInterface<PrometheusScraper>);
static_assert(Prometheus::hashdex::HashdexInterface<OpenMetricsScraper>);

}  // namespace PromPP::WAL::hashdex::scraper
