#include "head_wal.h"

#include <variant>

#include "exception.hpp"
#include "hashdex.hpp"
#include "head/lss.h"
#include "head/series_data.h"
#include "metrics/global_metrics.h"
#include "primitives/go_slice.h"
#include "wal/decoder.h"
#include "wal/encoder.h"
#include "wal/wal.h"

using Encoder = PromPP::WAL::GenericEncoder<PromPP::WAL::BasicEncoder<entrypoint::head::QueryableEncodingBimap&>>;
using EncoderPtr = std::unique_ptr<Encoder>;
using Decoder = PromPP::WAL::GenericDecoder<entrypoint::head::QueryableEncodingBimap&>;
using DecoderPtr = std::unique_ptr<Decoder>;
static_assert(sizeof(EncoderPtr) == sizeof(void*));
static_assert(sizeof(DecoderPtr) == sizeof(void*));

extern "C" void prompp_head_wal_encoder_ctor(void* args, void* res) {
  using entrypoint::head::LssVariantPtr;

  struct Arguments {
    uint16_t shard_id;
    uint8_t log_shards;
    LssVariantPtr lss;
  };

  struct Result {
    EncoderPtr encoder;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
  new (res) Result{.encoder = std::make_unique<Encoder>(lss, in->shard_id, in->log_shards)};
}

extern "C" void prompp_head_wal_encoder_ctor_from_decoder(void* args, void* res) {
  struct Arguments {
    DecoderPtr decoder;
  };

  struct Result {
    EncoderPtr encoder;
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto& generic_decoder = static_cast<Arguments*>(args)->decoder;
  const auto out = new (res) Result();

  if (generic_decoder->decoder().encoder_version() != PromPP::WAL::Writer::version) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    err_stream << "invalid encoder version" << std::endl;
    return;
  }

  const auto& decoder = generic_decoder->decoder();
  out->encoder = std::make_unique<Encoder>(decoder.sample_decoder().gorilla(), generic_decoder->label_set(), decoder.shard_id(),
                                           decoder.pow_two_of_total_shards(), decoder.last_processed_segment() + 1, decoder.sample_decoder().timestamp_base);
}

extern "C" void prompp_head_wal_encoder_dtor(void* args) {
  struct Arguments {
    EncoderPtr encoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_head_wal_encoder_add_inner_series(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries> incoming_inner_series;
    EncoderPtr encoder;
  };

  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
    uint32_t samples;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    in->encoder->add_inner_series(in->incoming_inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_head_wal_encoder_finalize(void* args, void* res) {
  struct Arguments {
    EncoderPtr encoder;
  };

  struct Result {
    PromPP::Primitives::Go::Slice<char> segment;
    PromPP::Primitives::Go::Slice<char> error;
    uint32_t samples;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  auto out_stream = PromPP::Primitives::Go::BytesStream(&out->segment);

  try {
    in->encoder->finalize(out, out_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_head_wal_decoder_ctor(void* args, void* res) {
  using entrypoint::head::LssVariantPtr;

  struct Arguments {
    LssVariantPtr lss;
    PromPP::WAL::BasicEncoderVersion encoder_version;
  };

  using Result = struct {
    DecoderPtr decoder;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
  new (res) Result{.decoder = std::make_unique<Decoder>(lss, in->encoder_version)};
}

extern "C" void prompp_head_wal_decoder_dtor(void* args) {
  struct Arguments {
    DecoderPtr decoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_head_wal_decoder_decode(void* args, void* res) {
  struct Arguments {
    DecoderPtr decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  struct Result {
    int64_t created_at;
    int64_t encoded_at;
    uint32_t samples;
    uint32_t series;
    uint32_t segment_id;
    PromPP::Primitives::Timestamp earliest_block_sample;
    PromPP::Primitives::Timestamp latest_block_sample;
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    in->inner_series->clear();
    in->decoder->decode_to_inner_series(in->segment, *in->inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_head_wal_decoder_decode_to_data_storage(void* args, void* res) {
  struct Arguments {
    DecoderPtr decoder;
    PromPP::Primitives::Go::SliceView<char> segment;
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  struct Result {
    PromPP::Primitives::Timestamp create_timestamp;
    PromPP::Primitives::Timestamp encode_timestamp;
    PromPP::Primitives::Go::Slice<char> error;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  try {
    metrics::MemoryAllocationsManualCalculator calculator(metrics::global_metrics()->data_storage_allocations);
    in->decoder->decode(in->segment,
                        [in, &calculator](PromPP::Primitives::LabelSetID ls_id, PromPP::Primitives::Timestamp timestamp, double value) PROMPP_LAMBDA_INLINE {
                          calculator.start();
                          in->encoder_wrapper->encoder.encode(ls_id, timestamp, value);
                        });
    out->create_timestamp = in->decoder->decoder().created_at_tsns();
    out->encode_timestamp = in->decoder->decoder().encoded_at_tsns();
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}
