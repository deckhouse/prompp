#include "wal_decoder.h"
#include "annotations.h"

#include "entrypoint/types/exception.h"
#include "entrypoint/types/hashdex.h"
#include "entrypoint/types/lss.h"
#include "primitives/go_slice.h"
#include "primitives/go_slice_protozero.h"
#include "wal/decoder.h"
#include "wal/output_decoder.h"

/**
 * @brief Construct a new WAL Decoder
 *
 * @param args {
 *     encoder_version uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_ctor(void* args, void* res) {
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

/**
 * @brief Destroy decoder
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Decoder* decoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->decoder;
}

/**
 * @brief Decode WAL-segment into protobuf message
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     created_at int64  // timestamp in ns when data was start writed to encoder
 *     encoded_at int64  // timestamp in ns when segment was encoded
 *     samples    uint32 // number of samples in segment
 *     series     uint32 // number of series in segment
 *     segment_id uint32 // processed segment id
 *     earliest_block_sample int64 // min timestamp in block
 *     latest_block_sample inte64 // max timestamp in block
 *     protobuf   []byte // decoded RemoteWrite protobuf content
 *     error      []byte // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_decode(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex
 *
 * @param args {
 *     decoder               uintptr // pointer to constructed decoder
 *     segment               []byte  // segment content
 * }
 * @param res {
 *     created_at            int64   // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64   // timestamp in ns when segment was encoded
 *     samples               uint32  // number of samples in segment
 *     series                uint32  // number of series in segment
 *     segment_id            uint32  // processed segment id
 *     earliest_block_sample int64   // min timestamp in block
 *     latest_block_sample   inte64  // max timestamp in block
 *     hashdex               uintptr // pointer to filled hashdex
 *     cluster               string  // value of label cluster from first sample
 *     replica               string  // value of label __replica__ from first sample
 *     error                 []byte  // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_decode_to_hashdex(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex with metadata for injection metrics.
 *
 * @param args {
 *     decoder               uintptr        // pointer to constructed decoder
 *     meta                  *MetaInjection // pointer to metadata for injection metrics.
 *     segment               []byte         // segment content
 * }
 * @param res {
 *     created_at            int64          // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64          // timestamp in ns when segment was encoded
 *     samples               uint32         // number of samples in segment
 *     series                uint32         // number of series in segment
 *     segment_id            uint32         // processed segment id
 *     earliest_block_sample int64          // min timestamp in block
 *     latest_block_sample   inte64         // max timestamp in block
 *     hashdex               uintptr        // pointer to filled hashdex
 *     cluster               string         // value of label cluster from first sample
 *     replica               string         // value of label __replica__ from first sample
 *     error                 []byte         // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_decode_to_hashdex_with_metric_injection(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Decode WAL-segment and drop decoded data
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     segment_id uint32  // last decoded segment id
 *     error   []byte     // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_decode_dry(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Decode all segments from given stream dump
 *
 * @param args {
 *     decoder    uintptr // pointer to constructed decoder
 *     stream     []byte  // stream dump
 *     segment_id uint32  // id of last segment to decode
 * }
 * @param res {
 *     offset     uint64 // number of read bytes from dump
 *     segment_id uint32 // last decoded segment id
 *     error      []byte // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_decoder_restore_from_stream(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

//
// OutputDecoder
//

using entrypoint::types::LssVariantPtr;

using OutputDecoder = PromPP::WAL::OutputDecoder<entrypoint::types::EncodingBimap>;
using OutputDecoderPtr = std::unique_ptr<OutputDecoder>;

static_assert(sizeof(OutputDecoderPtr) == sizeof(void*));

/**
 * @brief Construct a segment samples storage list
 *
 * @param args {
 *     count       uint64 // storages count
 *     storageList *SegmentSamplesStorageList
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_segment_samples_storage_list_ctor(void* args) {
  struct Arguments {
    uint64_t count;
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
  };

  const auto in = static_cast<Arguments*>(args);
  std::construct_at(in->storage_list, in->count);
}

/**
 * @brief Add sample to sample storage list
 *
 * @param args {
 *     samplesStorage *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 *     lsId           uint32 // label set id
 *     int64          timestamp // sample timestamp
 *     value          float64   // sample value
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_segment_samples_storage_add(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorage* samples_storage;
    PromPP::Primitives::LabelSetID ls_id;
    PromPP::Primitives::Timestamp timestamp;
    double value;
  };

  const auto in = static_cast<Arguments*>(args);
  in->samples_storage->add(in->ls_id, PromPP::Primitives::Sample(in->timestamp, in->value));
}

/**
 * @brief Clear sample storage list
 *
 * @param args {
 *     samplesStorage *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_segment_samples_storage_clear(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorage* samples_storage;
  };

  static_cast<Arguments*>(args)->samples_storage->clear();
}

/**
 * @brief Destroy segment samples storage list
 *
 * @param args {
 *     storageList *SegmentSamplesStorageList
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_segment_samples_storage_list_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
  };

  std::destroy_at(static_cast<Arguments*>(args)->storage_list);
}

/**
 * @brief Split storage list into messages by samples per message
 *
 * @param args {
 *     storageList                *SegmentSamplesStorageList
 *     message_samples_threshold  uint32
 *     messages                   []GoMessage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_segment_samples_storage_list_split_messages(void* args) {
  struct Arguments {
    PromPP::WAL::SegmentSamplesStorageList* storage_list;
    uint32_t message_samples_threshold;
    PromPP::Primitives::Go::Slice<PromPP::WAL::GoMessage> messages;
  };

  const auto in = static_cast<Arguments*>(args);
  PromPP::WAL::split_messages(*in->storage_list, in->message_samples_threshold, in->messages);
}

/**
 * @brief Construct a new WAL Output Decoder
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     output_lss          uintptr // pointer to constructed output label sets;
 *     encoder_version     uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed output decoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_output_decoder_ctor(void* args, void* res) {
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
  auto& output_lss = std::get<entrypoint::types::EncodingBimap>(*in->output_lss);
  new (res) Result{.decoder = std::make_unique<OutputDecoder>(*in->stateless_relabeler, output_lss, in->external_labels,
                                                              static_cast<PromPP::WAL::BasicEncoderVersion>(in->encoder_version))};
}

/**
 * @brief Destroy output decoder
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_output_decoder_dtor(void* args) {
  struct Arguments {
    OutputDecoderPtr decoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Dump output decoder state(output_lss and cache) to slice byte.
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     dump                []byte  // stream dump
 *     error               []byte  // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_output_decoder_dump_to(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief Load from dump(slice byte) output decoder state(output_lss and cache).
 *
 * @param args {
 *     dump                []byte  // stream dump
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     error               []byte  // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_output_decoder_load_from(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}

/**
 * @brief decode segment to slice RefSample.
 *
 * @param args {
 *     segment               []byte                 // segment content
 *     decoder               uintptr                // pointer to constructed output decoder
 *     samplesStorage        *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 *     lower_limit_timestamp int64                  // lower limit timestamp
 * }
 *
 * @param res {
 *     max_timestamp         int64       // max timestamp in slice RefSample
 *     outdated_sample_count uint32      // count of dropped samples on outdated
 *     dropped_sample_count  uint32      // count of dropped samples on relabeling rules
 *     add_series_count      uint32      // count of add series on relabeling rules
 *     dropped_series_count  uint32      // count of dropped series on relabeling rules
 *     sample_count         uint32       // count of samples added to samplesStorage
 *     error                 []byte      // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_output_decoder_decode(void* args, void* res) {
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
    entrypoint::types::handle_current_exception(err_stream);
  }
}
