#include <gtest/gtest.h>

#include "bare_bones/vector.h"
#include "go_head.h"
#include "primitives/snug_composites.h"
#include "primitives/timeseries.h"
#include "series_data/encoder.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Primitives::TimeseriesSemiview;
using PromPP::WAL::hashdex::GoHead;
using series_data::DataStorage;
using series_data::Encoder;

struct HashdexItem {
  size_t hash;
  TimeseriesSemiview timeseries;

  bool operator==(const HashdexItem& rhs) const noexcept = default;
};

class GoHeadFixture : public ::testing::Test {
 protected:
  using Lss = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>;

  GoHead<Lss> hashdex_;
  Lss lss_;
  DataStorage data_storage_;
  Encoder<> encoder_{data_storage_};
};

TEST_F(GoHeadFixture, Test) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 200, 2.0);

  lss_.find_or_emplace(LabelViewSet{{"job2", "cron2"}});
  encoder_.encode(1, 100, 1.1);

  std::vector<HashdexItem> actual;

  // Act
  hashdex_.presharding(&lss_, &data_storage_);
  std::ranges::for_each(hashdex_.metrics(), [&actual](const GoHead<Lss>::Iterator& it) {
    auto& item = actual.emplace_back();
    item.hash = it.hash();
    it.read(item.timeseries);
  });

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      std::vector{
          HashdexItem{
              .hash = 8789024106558160196ULL,
              .timeseries =
                  {
                      LabelViewSet{{"job", "cron"}},
                      BareBones::Vector{Sample(100, 1.0), Sample(200, 2.0)},
                  },
          },
          HashdexItem{
              .hash = 15789046129455230085ULL,
              .timeseries =
                  {
                      LabelViewSet{{"job2", "cron2"}},
                      BareBones::Vector{Sample(100, 1.1)},
                  },
          },
      },
      actual));
}

}  // namespace