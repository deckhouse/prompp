#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/last_over_time.h"
#include "series_data/decoder/decorator/lookback_delta_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::TimeInterval;
using PromPP::Primitives::Timestamp;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::kInvalidTimestamp;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::LastOverTimeWithStaleNansIterator;
using series_data::decoder::decorator::LookbackDeltaIterator;
using series_data::encoder::Sample;

constexpr Sample kInvalidSample{.timestamp = kInvalidTimestamp, .value = STALE_NAN};

struct LookbackDeltaIteratorCase {
  std::vector<Sample> samples;
  std::vector<TimeInterval> intervals;
  Timestamp lookback_delta{};
  std::vector<Sample> expected{};
};

class LookbackDeltaIteratorFixture : public testing::TestWithParam<LookbackDeltaIteratorCase> {
 protected:
  using Iterator = LookbackDeltaIterator<LastOverTimeWithStaleNansIterator<>>;

  DataStorage storage_;
  Encoder<> encoder_{storage_};

  void SetUp() override {
    for (const auto& sample : GetParam().samples) {
      encoder_.encode(0, sample.timestamp, sample.value);
    }
  }

  std::vector<Sample> get_samples(const std::vector<TimeInterval>& intervals, Timestamp lookback_delta) {
    std::vector<Sample> samples;

    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(
        storage_, storage_.open_chunks[0], [&samples, &intervals, lookback_delta]<typename DecodeIterator>(DecodeIterator&& begin, auto&&) {
          Iterator it(LastOverTimeWithStaleNansIterator(UniversalDecodeIterator{std::in_place_type<DecodeIterator>, std::forward<DecodeIterator>(begin)},
                                                        intervals.front()),
                      lookback_delta);
          samples.emplace_back(*it);

          std::ranges::for_each(intervals.begin() + 1, intervals.end(), [&samples, &it](const auto& interval) {
            it.set_interval(interval);
            samples.emplace_back(*it);
          });
        });

    return samples;
  }
};

TEST_P(LookbackDeltaIteratorFixture, Test) {
  // Arrange

  // Act
  const auto actual_samples = get_samples(GetParam().intervals, GetParam().lookback_delta);

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                                   .intervals{{.min = 0, .max = 100}},
                                                                   .lookback_delta = 50,
                                                                   .expected{Sample{.timestamp = 100, .value = 1.0}}},
                                         LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                                   .intervals{{.min = 0, .max = 99}},
                                                                   .lookback_delta = 50,
                                                                   .expected{kInvalidSample}},
                                         LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                                   .intervals{{.min = 101, .max = 200}},
                                                                   .lookback_delta = 50,
                                                                   .expected{kInvalidSample}}));

INSTANTIATE_TEST_SUITE_P(StaleNan,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{
                                                                       Sample{.timestamp = 5, .value = STALE_NAN},
                                                                       Sample{.timestamp = 10, .value = 1.0},
                                                                       Sample{.timestamp = 20, .value = STALE_NAN},
                                                                       Sample{.timestamp = 51, .value = 1.1},
                                                                   },
                                                                   .intervals{{.min = 0, .max = 100}},
                                                                   .lookback_delta = 50,
                                                                   .expected{Sample{.timestamp = 51, .value = 1.1}}},
                                         LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}},
                                                                   .intervals{{.min = 0, .max = 100}},
                                                                   .lookback_delta = 50,
                                                                   .expected{kInvalidSample}},
                                         LookbackDeltaIteratorCase{.samples{
                                                                       Sample{.timestamp = 100, .value = 1.0},
                                                                       Sample{.timestamp = 101, .value = STALE_NAN},
                                                                       Sample{.timestamp = 299, .value = 2.0},
                                                                   },
                                                                   .intervals{
                                                                       {.min = 0, .max = 100},
                                                                       {.min = 101, .max = 200},
                                                                       {.min = 201, .max = 300},
                                                                   },
                                                                   .lookback_delta = 50,
                                                                   .expected{
                                                                       Sample{.timestamp = 100, .value = 1.0},
                                                                       kInvalidSample,
                                                                       Sample{.timestamp = 299, .value = 2.0},
                                                                   }}));

INSTANTIATE_TEST_SUITE_P(TimeInterval,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{
                                                                       Sample{.timestamp = 99, .value = 1.1},
                                                                       Sample{.timestamp = 100, .value = 1.0},
                                                                       Sample{.timestamp = 200, .value = 1.0},
                                                                       Sample{.timestamp = 201, .value = 1.1},
                                                                   },
                                                                   .intervals{{.min = 100, .max = 200}},
                                                                   .lookback_delta = 50,
                                                                   .expected{Sample{.timestamp = 200, .value = 1.0}}},
                                         LookbackDeltaIteratorCase{.samples{
                                                                       Sample{.timestamp = 100, .value = 1.0},
                                                                       Sample{.timestamp = 150, .value = 1.1},
                                                                       Sample{.timestamp = 200, .value = 1.2},
                                                                   },
                                                                   .intervals{{.min = 100, .max = 200}},
                                                                   .lookback_delta = 50,
                                                                   .expected{Sample{.timestamp = 200, .value = 1.2}}}));

INSTANTIATE_TEST_SUITE_P(LookbackDeltaBoundary,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 50, .value = 1.0}},
                                                                   .intervals{{.min = 100, .max = 200}},
                                                                   .lookback_delta = 50,
                                                                   .expected{kInvalidSample}}));

INSTANTIATE_TEST_SUITE_P(KeepSample,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 150, .value = 2.0}},
                                                                   .intervals{{.min = 100, .max = 200}, {.min = 201, .max = 300}},
                                                                   .lookback_delta = 151,
                                                                   .expected{Sample{.timestamp = 150, .value = 2.0}, Sample{.timestamp = 150, .value = 2.0}}},
                                         LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 150, .value = 2.0}},
                                                                   .intervals{{.min = 100, .max = 200}, {.min = 201, .max = 300}},
                                                                   .lookback_delta = 150,
                                                                   .expected{Sample{.timestamp = 150, .value = 2.0}, kInvalidSample}}));

INSTANTIATE_TEST_SUITE_P(DropSample,
                         LookbackDeltaIteratorFixture,
                         testing::Values(LookbackDeltaIteratorCase{.samples{Sample{.timestamp = 50, .value = 2.0}},
                                                                   .intervals{{.min = 0, .max = 100}},
                                                                   .lookback_delta = 50,
                                                                   .expected{kInvalidSample}}));

}  // namespace
