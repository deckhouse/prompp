#include "wal_hashdex.h"
#include "annotations.hpp"

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/exception.h"
#include "entrypoint/types/feature_flags.h"
#include "entrypoint/types/hashdex.h"
#include "primitives/go_slice.h"

namespace {

using PromPP::WAL::hashdex::scraper::OpenMetricsScraper;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;
using ScraperError = PromPP::WAL::hashdex::scraper::Error;

struct GoMetadata {
  PromPP::Primitives::Go::String metric_name;
  PromPP::Primitives::Go::String text;
  uint32_t type;

  template <class Metadata>
  explicit GoMetadata(const Metadata& metadata) : metric_name(metadata.metric_name()), text(metadata.text()), type(static_cast<uint32_t>(metadata.type())) {}

  explicit GoMetadata(const PromPP::WAL::hashdex::Metadata& metadata)
      : metric_name(metadata.metric_name), text(metadata.text), type(static_cast<uint32_t>(metadata.type)) {}
};

// Forwarding exports use these shared types to describe their payloads.
struct HashdexArguments {
  HashdexVariant* hashdex;
};

struct MetadataResult {
  PromPP::Primitives::Go::Slice<GoMetadata> metadata;
};

struct HashdexResult {
  HashdexVariant* hashdex;
};

struct ScraperArguments {
  HashdexVariant* hashdex;
  PromPP::Primitives::Go::SliceView<char> buffer;
  PromPP::Primitives::Timestamp default_timestamp;
};

struct ScraperResult {
  ScraperError error{ScraperError::kNoError};
  uint32_t scraped{};
};

template <class HashdexType>
void get_metadata(void* args, void* res) {
  using Arguments = HashdexArguments;
  using Result = MetadataResult;

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  const auto metadata = std::get<HashdexType>(*in->hashdex).metadata();
  out->metadata.reserve(metadata.size());
  for (auto& m : metadata) {
    out->metadata.emplace_back(m);
  }
}

//
// ProtobufHashdex
//

}  // namespace

/**
 * @brief Construct a new WAL Hashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_protobuf_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::hashdex::Limits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kProtobuf>, in->limits};
}

/**
 * @brief Destroy hashdex
 *
 * @param args {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_hashdex_dtor(void* args) {
  struct Arguments {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->hashdex;
}

/**
 * @brief Fill hashdex from compressed via snappy protobuf
 *
 * Hashdex only indexing protobuf and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex             uintptr // pointer to constructed hashdex
 *     compressed_protobuf []byte  // compressed via snappy RemoteWrite protobuf content
 * }
 * @param res {
 *     // this data is a view over protobuf memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_protobuf_hashdex_snappy_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<char> compressed_protobuf;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::hashdex::Protobuf>(*in->hashdex_variant);
    hashdex.snappy_presharding(static_cast<std::string_view>(in->compressed_protobuf), entrypoint::types::feature_flags().features().skip_no_samples_series);
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
 * @brief Get parsed metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_protobuf_hashdex_get_metadata(void* args, void* res) {
  using Result [[maybe_unused]] = MetadataResult;
  using Arguments [[maybe_unused]] = HashdexArguments;
  get_metadata<PromPP::WAL::hashdex::Protobuf>(args, res);
}

//
// GoModelHashdex
//

/**
 * @brief Construct a new WAL GoModelHashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_go_model_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::hashdex::Limits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kGoModel>, in->limits};
}

/**
 * @brief Fill hashdex from Go memory
 *
 * Hashdex only indexing go memory (model.TimeSeries) and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     data     []model.TimeSeries  // Go content
 * }
 * @param res {
 *     // this data is a view over go memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_go_model_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries> data;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::hashdex::GoModel>(*in->hashdex_variant);
    hashdex.presharding(in->data);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::types::handle_current_exception(err_stream);
  }
}

namespace {

template <size_t hashdex_type>
PROMPP_ALWAYS_INLINE void scraper_hashdex_ctor(void* res) {
  using Result = HashdexResult;

  new (res) Result{.hashdex = new HashdexVariant{std::in_place_index<hashdex_type>}};
}

template <class Scraper>
PROMPP_ALWAYS_INLINE void scraper_hashdex_parse(void* args, void* res) {
  using Arguments = ScraperArguments;
  using Result = ScraperResult;

  const auto in = static_cast<Arguments*>(args);
  auto& scraper = std::get<Scraper>(*in->hashdex);
  const auto buffer = std::span{const_cast<char*>(in->buffer.data()), in->buffer.size()};
  ScraperError error;
  if (entrypoint::types::feature_flags().features().scraper_validate_utf_per_token) [[unlikely]] {
    error = scraper.parse_validate_utf_per_token(buffer, in->default_timestamp);
  } else [[likely]] {
    error = scraper.parse(buffer, in->default_timestamp);
  }
  new (res) Result{.error = error, .scraped = static_cast<uint32_t>(scraper.floats().size())};
}

template <class Scraper>
PROMPP_ALWAYS_INLINE void scraper_hashdex_get_metadata(void* args, void* res) {
  get_metadata<Scraper>(args, res);
}

}  // namespace

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on Prometheus parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_prometheus_scraper_hashdex_ctor(void* res) {
  using Result [[maybe_unused]] = HashdexResult;
  scraper_hashdex_ctor<HashdexType::kPrometheusScraper>(res);
}

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error   uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 *     scraped uint32 // number of parsed samples
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_prometheus_scraper_hashdex_parse(void* args, void* res) {
  using Result [[maybe_unused]] = ScraperResult;
  using Arguments [[maybe_unused]] = ScraperArguments;
  scraper_hashdex_parse<PrometheusScraper>(args, res);
}

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_prometheus_scraper_hashdex_get_metadata(void* args, void* res) {
  using Result [[maybe_unused]] = MetadataResult;
  using Arguments [[maybe_unused]] = HashdexArguments;
  scraper_hashdex_get_metadata<PrometheusScraper>(args, res);
}

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on OpenMetrics parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_open_metrics_scraper_hashdex_ctor(void* res) {
  using Result [[maybe_unused]] = HashdexResult;
  scraper_hashdex_ctor<HashdexType::kOpenMetricsScraper>(res);
}

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error   uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 *     scraped uint32 // number of parsed samples
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_open_metrics_scraper_hashdex_parse(void* args, void* res) {
  using Result [[maybe_unused]] = ScraperResult;
  using Arguments [[maybe_unused]] = ScraperArguments;
  scraper_hashdex_parse<OpenMetricsScraper>(args, res);
}

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_open_metrics_scraper_hashdex_get_metadata(void* args, void* res) {
  using Result [[maybe_unused]] = MetadataResult;
  using Arguments [[maybe_unused]] = HashdexArguments;
  scraper_hashdex_get_metadata<OpenMetricsScraper>(args, res);
}

/**
 * @brief Construct a new PromPP::WAL::hashdex::GoHead hashdex
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_go_head_hashdex_ctor(void* res) {
  struct Result {
    HashdexVariant* hashdex;
  };

  new (res) Result{.hashdex = new HashdexVariant{std::in_place_index<HashdexType::kGoHead>}};
}

/**
 * @brief Fill hashdex from Go Head
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     lss uintptr      // pointer to constructed lss
 *     dataStorage uintptr // pointer to constructed DataStorage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_wal_go_head_hashdex_presharding(void* args) {
  struct Arguments {
    HashdexVariant* hashdex;
    entrypoint::types::LssVariantPtr lss;
    entrypoint::types::DataStoragePtr data_storage;
  };

  const auto in = static_cast<Arguments*>(args);
  std::get<GoHeadHashdex>(*in->hashdex)
      .presharding(&std::get<entrypoint::types::QueryableEncodingBimap>(*in->lss), &std::get<entrypoint::types::DataStorageWithoutArenas>(*in->data_storage));
}
