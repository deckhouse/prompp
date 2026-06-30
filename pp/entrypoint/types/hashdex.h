#pragma once

#include <cstdint>
#include <variant>

#include "entrypoint/types/lss.h"
#include "wal/hashdex/basic_decoder.h"
#include "wal/hashdex/go_head.h"
#include "wal/hashdex/go_model.h"
#include "wal/hashdex/protobuf.h"
#include "wal/hashdex/scraper/scraper.h"

/**
 * used for indexing HashdexVariant.
 */
enum HashdexType : uint8_t {
  kProtobuf = 0,
  kGoModel,
  kDecoder,
  kPrometheusScraper,
  kOpenMetricsScraper,
  kGoHead,
};

using GoHeadHashdex = PromPP::WAL::hashdex::GoHead<entrypoint_types::QueryableEncodingBimap>;

using HashdexVariant = std::variant<PromPP::WAL::hashdex::Protobuf,
                                    PromPP::WAL::hashdex::GoModel,
                                    PromPP::WAL::hashdex::BasicDecoder,
                                    PromPP::WAL::hashdex::scraper::PrometheusScraper,
                                    PromPP::WAL::hashdex::scraper::OpenMetricsScraper,
                                    GoHeadHashdex>;
