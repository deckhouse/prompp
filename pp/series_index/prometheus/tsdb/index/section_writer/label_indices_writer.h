#pragma once

#include "prometheus/tsdb/index/stream_writer.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"

namespace series_index::prometheus::tsdb::index::section_writer {

template <class Lss, class Stream>
class LabelIndicesWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using StringWriter = PromPP::Prometheus::tsdb::index::StringWriter;
  using NoCrc32 = PromPP::Prometheus::tsdb::index::NoCrc32Tag;
  using IndexWriteContext = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>;

  LabelIndicesWriter(const Lss& lss, StreamWriter& writer) : lss_(lss), writer_(writer) {}

  void set_index_write_context(const IndexWriteContext* index_write_context) noexcept { index_write_context_ = index_write_context; }

  void write_label_indices() {
    assert(index_write_context_ != nullptr);
    indices_table_writer_.write_uint32<NoCrc32>(lss_.reverse_index().names_count());

    for (auto name_it = lss_.trie_index().names_trie().make_enumerative_iterator(); name_it.is_valid(); name_it.next()) {
      add_label_indices_table_item(name_it.key());
      write_label_index(name_it.value(), *lss_.trie_index().values_trie(name_it.value()));
    }
  }

  void write_label_indices_table() {
    const uint32_t payload_size = indices_table_writer_.writer().buffer().size();
    writer_.write_payload(payload_size, [this, payload_size]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write(indices_table_writer_.writer().buffer());
    });

    indices_table_writer_.writer().free_memory();
  }

 private:
  StringWriter indices_table_writer_;

  const Lss& lss_;
  const IndexWriteContext* index_write_context_{};
  StreamWriter& writer_;

  void add_label_indices_table_item(std::string_view name) {
    indices_table_writer_.write<NoCrc32>(0x01);

    indices_table_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(name.length()));
    indices_table_writer_.write<NoCrc32>(name);

    indices_table_writer_.write_varint<NoCrc32>(static_cast<uint64_t>(writer_.position()));
  }

  template <class Trie>
  void write_label_index(uint32_t name_id, const Trie& values_trie) {
    static constexpr uint32_t kNamesCount = 1;
    const uint32_t values_count = lss_.reverse_index().values_count(name_id);
    const uint32_t payload_size = sizeof(kNamesCount) + sizeof(values_count) + values_count * (sizeof(uint32_t));

    writer_.write_payload(payload_size, [&]() mutable {
      writer_.template write_uint32<NoCrc32>(payload_size);
      writer_.write_uint32(kNamesCount);
      writer_.write_uint32(values_count);

      for (auto value_it = values_trie.make_enumerative_iterator(); value_it.is_valid(); value_it.next()) {
        writer_.write_uint32(index_write_context_->symbol_ref_for_label_index_value(name_id, value_it.value()));
      }
    });
  }
};

}  // namespace series_index::prometheus::tsdb::index::section_writer