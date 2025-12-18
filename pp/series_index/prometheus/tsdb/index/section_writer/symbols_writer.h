#pragma once

#include "bare_bones/preprocess.h"
#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class Stream>
class SymbolsWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;

  SymbolsWriter(const Lss& lss, SymbolReferencesMap& symbol_references, StreamWriter& writer)
      : lss_(lss), symbol_references_(symbol_references), writer_(writer) {}

  void write() {
    generate_symbol_id_list();
    deduplicate_and_generate_references();
    write_symbols();
  }

 private:
  static constexpr SymbolLssId kEmptySymbol{};

  const Lss& lss_;
  SymbolReferencesMap& symbol_references_;
  StreamWriter& writer_;

  std::vector<SymbolLssId> symbol_ids_;
  uint32_t serialized_unique_symbols_length_ = 0;
  uint32_t unique_symbols_count_ = 0;

  void generate_symbol_id_list() {
    symbol_ids_.reserve(get_symbols_count() + 1);
    symbol_ids_.emplace_back(kEmptySymbol);

    const auto names = lss_.data_view().labels_keys();
    const auto values = lss_.data_view().labels_values();

    for (auto it = names.begin(), e = names.end(); it != e; ++it) {
      symbol_ids_.emplace_back(it.id());
    }

    for (auto it = values.begin(), e = values.end(); it != e; ++it) {
      symbol_ids_.emplace_back(it.key_id(), it.value_id());
    }

    std::ranges::sort(symbol_ids_, [this](SymbolLssId a, SymbolLssId b) PROMPP_LAMBDA_INLINE { return get_symbol(a) < get_symbol(b); });
  }

  void deduplicate_and_generate_references() {
    uint32_t symbol_index = 0;
    for (auto it = symbol_ids_.begin(); it != symbol_ids_.end(); ++symbol_index, ++unique_symbols_count_) {
      symbol_references_.try_emplace(*it, symbol_index);

      auto symbol = get_symbol(*it);
      serialized_unique_symbols_length_ += serialized_string_length(symbol);

      while (++it != symbol_ids_.end() && symbol == get_symbol(*it)) {
        symbol_references_.try_emplace(*it, symbol_index);
        it->mark_as_duplicated();
      }
    }
  }

  [[nodiscard]] uint32_t get_symbols_count() const noexcept {
    return lss_.data_view().labels_keys().size() + lss_.data_view().labels_values().size();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static uint32_t serialized_string_length(const std::string_view& str) noexcept {
    return BareBones::Encoding::VarInt::length(str.length()) + str.length();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view get_symbol(SymbolLssId symbol_id) const noexcept {
    if (symbol_id.is_empty()) [[unlikely]] {
      return {};
    }

    const auto view = lss_.data_view();
    if (symbol_id.is_name()) {
      return view.labels_keys()[symbol_id.name_id];
    }

    return view.labels_values().get_by_id(symbol_id.name_id, symbol_id.value_id);
  }

  void write_symbols() noexcept {
    const uint32_t payload_size = sizeof(unique_symbols_count_) + serialized_unique_symbols_length_;
    writer_.write_payload(payload_size, [this, payload_size]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write_uint32(unique_symbols_count_);

      for (auto symbol_id : symbol_ids_) {
        if (!symbol_id.is_duplicated()) {
          auto symbol = get_symbol(symbol_id);
          writer_.write_varint(static_cast<uint64_t>(symbol.length()));
          writer_.write(symbol);
        }
      }
    });
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer