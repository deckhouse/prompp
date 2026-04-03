#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/delta_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::DeltaIterator;
using series_data::encoder::Sample;

struct DeltaIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::TimeInterval interval;
  std::vector<Sample> expected{};
};

class DeltaIteratorFixture : public ::testing::TestWithParam<DeltaIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }
};

TEST_P(DeltaIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    std::ranges::copy(DeltaIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval),
                      DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

TEST_P(DeltaIteratorFixture, TestReset) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    Iterator begin_at_start = begin;
    DeltaIterator iterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval);
    std::advance(iterator, GetParam().samples.size());

    iterator = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin_at_start)};
    std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         DeltaIteratorFixture,
                         testing::Values(DeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                           .interval{.min = 0, .max = 100},
                                                           .expected{Sample{.timestamp = 100, .value = 1.0}}},
                                         DeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 0, .max = 99}, .expected{}},
                                         DeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval{.min = 101, .max = 200}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(
    StaleNan,
    DeltaIteratorFixture,
    testing::Values(DeltaIteratorCase{.samples{
                                          Sample{.timestamp = 5, .value = STALE_NAN},
                                          Sample{.timestamp = 10, .value = 1.0},
                                          Sample{.timestamp = 20, .value = STALE_NAN},
                                          Sample{.timestamp = 30, .value = 1.1},
                                      },
                                      .interval{.min = 0, .max = 100},
                                      .expected{Sample{.timestamp = 10, .value = 1.0}, Sample{.timestamp = 30, .value = 1.1}}},
                    DeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}}, .interval{.min = 0, .max = 100}, .expected{}}));

INSTANTIATE_TEST_SUITE_P(TimeInterval,
                         DeltaIteratorFixture,
                         testing::Values(DeltaIteratorCase{.samples{
                                                               Sample{.timestamp = 99, .value = 1.0},
                                                               Sample{.timestamp = 100, .value = 1.1},
                                                               Sample{.timestamp = 200, .value = 1.1},
                                                               Sample{.timestamp = 201, .value = 1.0},
                                                           },
                                                           .interval{.min = 100, .max = 200},
                                                           .expected{Sample{.timestamp = 100, .value = 1.1}, Sample{.timestamp = 200, .value = 1.1}}},
                                         DeltaIteratorCase{.samples{
                                                               Sample{.timestamp = 100, .value = 1.1},
                                                               Sample{.timestamp = 150, .value = 1.2},
                                                               Sample{.timestamp = 200, .value = 1.0},
                                                           },
                                                           .interval{.min = 100, .max = 200},
                                                           .expected{Sample{.timestamp = 100, .value = 1.1}, Sample{.timestamp = 200, .value = 1.0}}}));

}  // namespace