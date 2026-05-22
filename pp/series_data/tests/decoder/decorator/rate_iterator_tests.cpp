#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/rate_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::RateIterator;
using series_data::encoder::Sample;

struct RateIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::TimeInterval interval;
  std::vector<Sample> expected{};
};

class RateIteratorFixture : public ::testing::TestWithParam<RateIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }
};

TEST_P(RateIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    std::ranges::copy(RateIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval),
                      DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         RateIteratorFixture,
                         testing::Values(RateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                          .interval{.min = 0, .max = 100},
                                                          .expected{Sample{.timestamp = 100, .value = 1.0}}},
                                         RateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 0, .max = 99}, .expected{}},
                                         RateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 101, .max = 200}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(
    StaleNan,
    RateIteratorFixture,
    testing::Values(RateIteratorCase{.samples{
                                         Sample{.timestamp = 5, .value = STALE_NAN},
                                         Sample{.timestamp = 10, .value = 1.0},
                                         Sample{.timestamp = 20, .value = STALE_NAN},
                                         Sample{.timestamp = 30, .value = 2.0},
                                     },
                                     .interval{.min = 0, .max = 100},
                                     .expected{Sample{.timestamp = 10, .value = 1.0}, Sample{.timestamp = 30, .value = 2.0}}},
                    RateIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}}, .interval{.min = 0, .max = 100}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(TimeInterval,
                         RateIteratorFixture,
                         testing::Values(RateIteratorCase{.samples{
                                                              Sample{.timestamp = 99, .value = 1.0},
                                                              Sample{.timestamp = 100, .value = 1.0},
                                                              Sample{.timestamp = 200, .value = 1.0},
                                                              Sample{.timestamp = 201, .value = 1.0},
                                                          },
                                                          .interval{.min = 100, .max = 200},
                                                          .expected{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 200, .value = 1.0}}}));

INSTANTIATE_TEST_SUITE_P(FirstAndLastValueInInterval,
                         RateIteratorFixture,
                         testing::Values(RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 1.0},
                                                              Sample{.timestamp = 120, .value = 2.0},
                                                              Sample{.timestamp = 160, .value = 3.0},
                                                              Sample{.timestamp = 200, .value = 4.0},
                                                          },
                                                          .interval{.min = 100, .max = 200},
                                                          .expected{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 200, .value = 4.0}}}));

INSTANTIATE_TEST_SUITE_P(CounterResetting,
                         RateIteratorFixture,
                         testing::Values(RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 5.0},
                                                              Sample{.timestamp = 200, .value = 1.0},
                                                          },
                                                          .interval{.min = 100, .max = 300},
                                                          .expected{Sample{.timestamp = 100, .value = 5.0}, Sample{.timestamp = 200, .value = 1.0}}},
                                         RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 5.0},
                                                              Sample{.timestamp = 200, .value = 4.0},
                                                              Sample{.timestamp = 300, .value = 3.0},
                                                              Sample{.timestamp = 400, .value = 2.0},
                                                              Sample{.timestamp = 500, .value = 1.0},
                                                          },
                                                          .interval{.min = 100, .max = 500},
                                                          .expected{Sample{.timestamp = 100, .value = 5.0}, Sample{.timestamp = 200, .value = 4.0},
                                                                    Sample{.timestamp = 300, .value = 3.0}, Sample{.timestamp = 400, .value = 2.0},
                                                                    Sample{.timestamp = 500, .value = 1.0}}},
                                         RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 5.0},
                                                              Sample{.timestamp = 200, .value = 4.0},
                                                              Sample{.timestamp = 250, .value = 5.0},
                                                              Sample{.timestamp = 300, .value = 3.0},
                                                              Sample{.timestamp = 400, .value = 2.0},
                                                              Sample{.timestamp = 500, .value = 1.0},
                                                          },
                                                          .interval{.min = 100, .max = 500},
                                                          .expected{Sample{.timestamp = 100, .value = 5.0}, Sample{.timestamp = 200, .value = 4.0},
                                                                    Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 300, .value = 3.0},
                                                                    Sample{.timestamp = 400, .value = 2.0}, Sample{.timestamp = 500, .value = 1.0}}},
                                         RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 1.0},
                                                              Sample{.timestamp = 120, .value = 2.0},
                                                              Sample{.timestamp = 160, .value = 0.0},
                                                              Sample{.timestamp = 200, .value = 2.0},
                                                              Sample{.timestamp = 250, .value = 3.0},
                                                              Sample{.timestamp = 300, .value = 4.0},
                                                          },
                                                          .interval{.min = 100, .max = 300},
                                                          .expected{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 120, .value = 2.0},
                                                                    Sample{.timestamp = 160, .value = 0.0}, Sample{.timestamp = 300, .value = 4.0}}},
                                         RateIteratorCase{.samples{
                                                              Sample{.timestamp = 100, .value = 1.0},
                                                              Sample{.timestamp = 120, .value = 2.0},
                                                              Sample{.timestamp = 160, .value = 0.0},
                                                              Sample{.timestamp = 201, .value = 2.0},
                                                          },
                                                          .interval{.min = 100, .max = 200},
                                                          .expected{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 120, .value = 2.0},
                                                                    Sample{.timestamp = 160, .value = 0.0}}}));

}  // namespace