#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/irate_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::IRateIterator;
using series_data::encoder::Sample;

struct IRateIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::TimeInterval interval;
  std::vector<Sample> expected{};
};

class IRateIteratorFixture : public ::testing::TestWithParam<IRateIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }
};

TEST_P(IRateIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    std::ranges::copy(IRateIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval),
                      DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

TEST_P(IRateIteratorFixture, TestReset) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    Iterator begin_at_start = begin;
    IRateIterator iterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval);
    std::advance(iterator, GetParam().samples.size());

    iterator = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin_at_start)};
    std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         IRateIteratorFixture,
                         testing::Values(IRateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 0, .max = 100}, .expected{}},
                                         IRateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 0, .max = 99}, .expected{}},
                                         IRateIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 101, .max = 200}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(
    StaleNan,
    IRateIteratorFixture,
    testing::Values(IRateIteratorCase{.samples{
                                          Sample{.timestamp = 5, .value = STALE_NAN},
                                          Sample{.timestamp = 10, .value = 1.0},
                                          Sample{.timestamp = 15, .value = 1.0},
                                          Sample{.timestamp = 20, .value = STALE_NAN},
                                          Sample{.timestamp = 30, .value = 1.1},
                                      },
                                      .interval{.min = 0, .max = 100},
                                      .expected{Sample{.timestamp = 15, .value = 1.0}, Sample{.timestamp = 30, .value = 1.1}}},
                    IRateIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}}, .interval{.min = 0, .max = 100}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(TimeInterval,
                         IRateIteratorFixture,
                         testing::Values(IRateIteratorCase{.samples{
                                                               Sample{.timestamp = 99, .value = 1.0},
                                                               Sample{.timestamp = 100, .value = 1.1},
                                                               Sample{.timestamp = 200, .value = 1.1},
                                                               Sample{.timestamp = 201, .value = 1.0},
                                                           },
                                                           .interval{.min = 100, .max = 200},
                                                           .expected{Sample{.timestamp = 100, .value = 1.1}, Sample{.timestamp = 200, .value = 1.1}}},
                                         IRateIteratorCase{.samples{
                                                               Sample{.timestamp = 100, .value = 1.1},
                                                               Sample{.timestamp = 150, .value = 1.2},
                                                               Sample{.timestamp = 180, .value = 1.2},
                                                               Sample{.timestamp = 200, .value = 1.3},
                                                           },
                                                           .interval{.min = 100, .max = 200},
                                                           .expected{Sample{.timestamp = 180, .value = 1.2}, Sample{.timestamp = 200, .value = 1.3}}}));

}  // namespace