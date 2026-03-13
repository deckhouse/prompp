#pragma once

#include "primitives/go_slice.h"
#include "wal/output_decoder.h"

namespace entrypoint::head {

class SegmentSamplesStorageList {
 public:
  explicit SegmentSamplesStorageList(uint64_t count) : storage_list_(count) {}

 private:
  PromPP::Primitives::Go::Slice<PromPP::WAL::SegmentSamplesStorage> storage_list_;
  BareBones::Vector<PromPP::WAL::SegmentSamplesStorage> message_boundaries_;
};

};  // namespace entrypoint::head