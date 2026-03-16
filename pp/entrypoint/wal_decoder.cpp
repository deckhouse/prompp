#include "wal_decoder.h"

#include "exception.hpp"
#include "hashdex.hpp"
#include "head/lss.h"
#include "primitives/go_slice.h"
#include "primitives/go_slice_protozero.h"
#include "wal/decoder.h"
#include "wal/output_decoder.h"

extern "C" void prompp_wal_decoder_ctor(void* args, void* res) {
  struct Arguments {
    uint8_t encoder_version;
  };
  using Result = struct {
    PromPP::WAL::Decoder* decoder;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->decoder = new PromPP::WAL::Decoder(static_cast<PromPP::WAL::BasicEncoderVersion>(in->encoder_version));
}

extern "C" void prompp_wal_decoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

extern "C" void prompp_wal_decoder_decode(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    PromPP::Primitives::Go::Slice<char> protobuf;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode(in->segment, out->protobuf, *out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_to_hashdex(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::kDecoder>};
    auto& hashdex = std::get<PromPP::WAL::hashdex::BasicDecoder>(*out->hashdex_variant);
    in->decoder->decode_to_hashdex(in->segment, hashdex, *out);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_to_hashdex_with_metric_injection(void* args, void* res) {
  struct MetaInjection {
    std::chrono::system_clock::time_point now;
    std::chrono::nanoseconds sent_at{0};
    PromPP::Primitives::Go::String agent_uuid;
    PromPP::Primitives::Go::String hostname;

    [[nodiscard]] explicit PROMPP_ALWAYS_INLINE operator PromPP::WAL::hashdex::BasicDecoder::MetaInjection() const noexcept {
      return PromPP::WAL::hashdex::BasicDecoder::MetaInjection{
          .now = now,
          .sent_at = sent_at,
          .agent_uuid = static_cast<std::string_view>(agent_uuid),
          .hostname = static_cast<std::string_view>(hostname),
      };
    }
  };

  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    MetaInjection* meta;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  using Result = struct {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->hashdex_variant = new HashdexVariant{std::in_place_index<HashdexType::kDecoder>};
    auto& hashdex = std::get<PromPP::WAL::hashdex::BasicDecoder>(*out->hashdex_variant);
    in->decoder->decode_to_hashdex(in->segment, hashdex, *out, static_cast<PromPP::WAL::hashdex::BasicDecoder::MetaInjection>(*in->meta));
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_decoder_decode_dry(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
  };
  struct Result {
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->decode_dry(in->segment, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_decoder_restore_from_stream(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
    PromPP::Primitives::Go::SliceView<char> stream;
    uint32_t segment_id;
  };
  struct Result {
    size_t offset = 0;
    uint32_t segment_id;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->decoder->restore_from_stream(in->stream, in->segment_id, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

//
// OutputDecoder
//

using entrypoint::head::LssVariantPtr;

using OutputDecoder = PromPP::WAL::OutputDecoder<entrypoint::head::EncodingBimap>;
using OutputDecoderPtr = std::unique_ptr<OutputDecoder>;

static_assert(sizeof(OutputDecoderPtr) == sizeof(void*));

extern "C" void prompp_wal_segment_samples_storage_list_ctor(void* args) {
  struct Arguments {
    uint64_t count;
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
  };

  const auto in = static_cast<Arguments*>(args);
  std::construct_at(in->storage_list, in->count);
}

extern "C" void prompp_wal_segment_samples_storage_add(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorage* samples_storage;
    PromPP::Primitives::LabelSetID ls_id;
    PromPP::Primitives::Timestamp timestamp;
    double value;
  };

  const auto in = static_cast<Arguments*>(args);
  in->samples_storage->add(in->ls_id, PromPP::Primitives::Sample(in->timestamp, in->value));
}

extern "C" void prompp_wal_segment_samples_storage_clear(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorage* samples_storage;
  };

  static_cast<Arguments*>(args)->samples_storage->clear();
}

extern "C" void prompp_wal_segment_samples_storage_list_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
  };

  std::destroy_at(static_cast<Arguments*>(args)->storage_list);
}

extern "C" void prompp_wal_segment_samples_storage_list_split_messages(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
    uint32_t samples_per_message;
  };
  struct Result {
    uint32_t messages_count;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);
  out->messages_count = in->storage_list->split_messages(in->samples_per_message);
}

extern "C" void prompp_wal_output_decoder_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    LssVariantPtr output_lss;
    uint8_t encoder_version;
  };
  using Result = struct {
    OutputDecoderPtr decoder;
  };

  auto* in = static_cast<Arguments*>(args);
  auto& output_lss = std::get<entrypoint::head::EncodingBimap>(*in->output_lss);
  new (res) Result{.decoder = std::make_unique<OutputDecoder>(*in->stateless_relabeler, output_lss, in->external_labels,
                                                              static_cast<PromPP::WAL::BasicEncoderVersion>(in->encoder_version))};
}

extern "C" void prompp_wal_output_decoder_dtor(void* args) {
  struct Arguments {
    OutputDecoderPtr decoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_wal_output_decoder_dump_to(void* args, void* res) {
  struct Arguments {
    OutputDecoderPtr decoder;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<char> dump;
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    PromPP::Primitives::Go::BytesStream bytes_stream{&out->dump};
    in->decoder->dump_to(bytes_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_output_decoder_load_from(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<char> dump;
    OutputDecoderPtr decoder;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);
  auto* out = new (res) Result();

  try {
    std::ispanstream bytes_stream(static_cast<std::string_view>(in->dump));
    in->decoder->load_from(bytes_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_output_decoder_decode(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<char> segment;
    OutputDecoderPtr decoder;
    PromPP::WAL::SegmentSamplesStorage* samples_storage;
    int64_t lower_limit_timestamp;
  };

  struct Result {
    int64_t max_timestamp{};
    uint32_t outdated_sample_count{};
    uint32_t dropped_sample_count{};
    uint32_t add_series_count{};
    uint32_t dropped_series_count{};
    uint32_t sample_count{};
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto* in = static_cast<Arguments*>(args);
  auto* out = new (res) Result();

  try {
    std::ispanstream{static_cast<std::string_view>(in->segment)} >> *in->decoder;
    out->add_series_count = in->decoder->add_series_count();
    out->dropped_series_count = in->decoder->dropped_series_count();
    uint32_t prev_sample_count = in->samples_storage->samples_count();
    in->decoder->process_segment([in, out](PromPP::Primitives::LabelSetID ls_id, PromPP::Primitives::Timestamp ts, PromPP::Primitives::Sample::value_type v,
                                           bool is_dropped) PROMPP_LAMBDA_INLINE {
      if (is_dropped) {
        // skip dropped sample
        ++out->dropped_sample_count;
        return;
      }

      if (ts < in->lower_limit_timestamp) {
        // skip sample lower limit timestamp
        ++out->outdated_sample_count;
        return;
      }

      out->max_timestamp = std::max(out->max_timestamp, ts);
      in->samples_storage->add(ls_id, PromPP::Primitives::Sample(ts, v));
    });
    out->sample_count = in->samples_storage->samples_count() - prev_sample_count;
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}
