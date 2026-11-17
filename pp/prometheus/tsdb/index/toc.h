#pragma once

#include "bare_bones/preprocess.h"
#include "types.h"

namespace PromPP::Prometheus::tsdb::index {

struct PROMPP(packed) Toc {
  using Reference = uint64_t;

  Reference symbols{};
  Reference series{};
  Reference label_indices{};
  Reference label_indices_table{};
  Reference postings{};
  Reference postings_offset_table{};
};

}  // namespace PromPP::Prometheus::tsdb::index