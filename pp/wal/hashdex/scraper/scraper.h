#pragma once

#include <simdutf/simdutf.h>

#include <span>

#include "bare_bones/algorithm.h"
#include "bare_bones/vector.h"
#include "bare_bones/xxhash.h"
#include "encoding.h"
#include "marked.h"
#include "parser.h"
#include "prometheus/hashdex.h"
#include "prometheus/metric.h"
#include "prometheus/textparse/escape.h"
#include "prometheus/value.h"

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

  void encode_metric_data(const uint32_t metric_offset) noexcept {
    metric_buffer_.add_metric(metric_offset);

    const auto base_buffer = parser_.tokenizer().buffer();
    sort_and_filter_labels(base_buffer);

    metric_buffer_.bytes_enlarge(encoding::metric_maximum_encoding_size(labels_.size()));

    const encoding::LayoutMarker layout =
        encoding::LayoutMarker::make(marked_sample_.has_ts, labels_.size(), encoding::SampleCodec::value_type(marked_sample_.sample.value()));
    metric_buffer_.add_layout_and_count(layout, labels_.size());

    append_labels_hash_and_encode(metric_offset, base_buffer);
    metric_buffer_.add_sample(layout, marked_sample_.sample);
  }

  void sort_and_filter_labels(std::string_view base_buffer) noexcept {
    const auto it = std::remove_if(labels_.begin(), labels_.end(), [](const MarkedLabel& label) PROMPP_LAMBDA_INLINE { return label.value.is_empty(); });
    labels_.erase(it, labels_.end());

    const auto compare = [buffer = base_buffer](const MarkedLabel& a, const MarkedLabel& b)
                             PROMPP_LAMBDA_INLINE { return a.name.view(buffer) < b.name.view(buffer); };

    std::sort(labels_.begin(), labels_.end(), compare);
  }

  void append_labels_hash_and_encode(uint32_t metric_offset, std::string_view base_buffer) noexcept {
    BareBones::XXHash3 hash;
    for (const auto& source_label : labels_) {
      hash.extend(source_label.name.view(base_buffer), source_label.value.view(base_buffer));

      MarkedLabel encoded_label = source_label;
      if (!encoded_label.name.is_reserved_name()) [[likely]] {
        encoded_label.name.offset -= metric_offset;
      }
      encoded_label.value.offset -= metric_offset;
      metric_buffer_.add_label(encoded_label);
    }

    metric_buffer_.add_hash(hash.hash());
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
