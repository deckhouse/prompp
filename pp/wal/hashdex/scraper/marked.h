#pragma once

#include <string_view>

#include "bare_bones/vector.h"
#include "encoding.h"
#include "primitives/primitives.h"
#include "prometheus/metric.h"
#include "prometheus/value.h"

namespace PromPP::WAL::hashdex::scraper::inline marked {

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

class Metric {
 public:
  using MarkedT = MarkedMetric;

  struct Context {
    std::string_view buffer;
    const BareBones::Memory<BareBones::MemoryControlBlock, char>& bytes_buffer;
    Primitives::Timestamp default_timestamp{};
  };

  Metric(const Context& ctx, const MarkedMetric* item)
      : buffer_(ctx.buffer), bytes_buffer_(ctx.bytes_buffer), item_(item), default_timestamp_(ctx.default_timestamp) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const MarkedMetric* item() const noexcept { return item_; }
  PROMPP_ALWAYS_INLINE void set_item(const MarkedMetric* item) noexcept { item_ = item; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return item_->hash; }

  template <class Timeseries>
  void read(Timeseries& ts) const {
    const char* ptr = bytes_buffer_.control_block().data + item_->data_offset;

    const auto [next_ptr, layout, labels_count] = encoding::LayoutCountCodec::decode(ptr);
    ptr = next_ptr;

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
  const BareBones::Memory<BareBones::MemoryControlBlock, char>& bytes_buffer_;
  const MarkedMetric* item_{};
  Primitives::Timestamp default_timestamp_;
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
    return {typename Base::Context{buffer, bytes_buffer_, default_ts}, Base::buffer_.data(), Base::items_count()};
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

  void bytes_enlarge(uint32_t extra_bytes) noexcept {
    const uint32_t offset = bytes_count();

    bytes_buffer_.grow_to_fit_at_least(offset + extra_bytes);

    bytes_ptr_ = bytes_buffer_.control_block().data + offset;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return Base::buffer_.allocated_memory() + bytes_buffer_.allocated_memory(); }

  PROMPP_ALWAYS_INLINE void initialize(size_t reserve_bytes) noexcept {
    this->buffer_.clear();

    const size_t bytes_buffer_reserve = (reserve_bytes / 3) * 2;
    const size_t items_buffer_reserve = (bytes_buffer_reserve / 3) / sizeof(MarkedMetric);

    this->buffer_.reserve(items_buffer_reserve);
    bytes_buffer_.resize_to_fit_at_least(bytes_buffer_reserve);
    bytes_ptr_ = bytes_buffer_.control_block().data;
  }

  PROMPP_ALWAYS_INLINE void add_hash(uint64_t hash) noexcept { this->buffer_.back().hash = hash; }

  PROMPP_ALWAYS_INLINE void add_metric(uint32_t global_offset) noexcept {
    this->buffer_.push_back(MarkedMetric{.hash = {}, .base_offset = global_offset, .data_offset = bytes_count()});
  }

  void add_layout_and_count(const encoding::LayoutMarker layout, const uint32_t count) noexcept {
    bytes_ptr_ = encoding::LayoutCountCodec::encode(bytes_ptr_, layout, count);
  }

  void add_label(MarkedLabel label) noexcept {
    bytes_ptr_ = encoding::LabelCodec::encode(bytes_ptr_, label.name.offset, label.name.length, label.value.offset, label.value.length);
  }

  void add_sample(encoding::LayoutMarker layout, const Primitives::Sample& sample) noexcept {
    using encoding::SampleValueType;

    bytes_ptr_ = encoding::SampleCodec::encode(bytes_ptr_, layout, sample);
  }

  void add_padding() noexcept {
    constexpr size_t kPaddingSizeBytes = 16;
    bytes_enlarge(kPaddingSizeBytes);
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t bytes_count() const noexcept { return bytes_ptr_ - bytes_buffer_.control_block().data; }

  BareBones::Memory<BareBones::MemoryControlBlock, char> bytes_buffer_;
  char* bytes_ptr_{};
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

}  // namespace PromPP::WAL::hashdex::scraper::inline marked