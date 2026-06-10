#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class Stream>
class SymbolsWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;
  using IndexWriteContext = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>;

  SymbolsWriter(const IndexWriteContext& index_write_context, StreamWriter& writer) : index_write_context_(index_write_context), writer_(writer) {}

  void write() {
    calculate_serialized_size();
    write_symbols();
  }

 private:
  const IndexWriteContext& index_write_context_;
  StreamWriter& writer_;

  uint32_t serialized_unique_symbols_length_ = 0;
  uint32_t unique_symbols_count_ = 0;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t serialized_string_length(const std::string_view& str) noexcept {
    return BareBones::Encoding::VarInt::length(str.length()) + str.length();
  }

  void calculate_serialized_size() {
    serialized_unique_symbols_length_ = 0;
    unique_symbols_count_ = 0;
    index_write_context_.for_each_symbol([this](uint32_t, std::string_view symbol) {
      serialized_unique_symbols_length_ += serialized_string_length(symbol);
      ++unique_symbols_count_;
    });
  }

  void write_symbols() noexcept {
    const uint32_t payload_size = sizeof(unique_symbols_count_) + serialized_unique_symbols_length_;
    writer_.write_payload(payload_size, [this, payload_size]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write_uint32(unique_symbols_count_);

      index_write_context_.for_each_symbol([this](uint32_t, std::string_view symbol) {
        writer_.write_varint(static_cast<uint64_t>(symbol.length()));
        writer_.write(symbol);
      });
    });
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer