#include "go_constants.h"

#include "prometheus/relabeler.h"

namespace {

static_assert(sizeof(std::vector<char>) == Sizeof_StdVector);
static_assert(sizeof(BareBones::Vector<char>) == Sizeof_BareBonesVector);
static_assert(sizeof(roaring::Roaring) == Sizeof_RoaringBitset);

static_assert(sizeof(PromPP::Prometheus::Relabel::InnerSeries) == Sizeof_InnerSeries);

}  // namespace