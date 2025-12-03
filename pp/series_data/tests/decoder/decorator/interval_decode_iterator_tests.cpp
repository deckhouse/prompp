#include <gtest/gtest.h>

#include "series_data/decoder.h"
#include "series_data/decoder/decorator/interval_decode_iterator.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::IntervalDecodeIterator;
using series_data::encoder::Sample;

struct IntervalDecodeIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::Timestamp interval;
  std::vector<Sample> expected{};
};

class IntervalDecodeIteratorFixture : public ::testing::TestWithParam<IntervalDecodeIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }
};

TEST_P(IntervalDecodeIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    std::ranges::copy(IntervalDecodeIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval),
                      DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(
    OneSample,
    IntervalDecodeIteratorFixture,
    testing::Values(
        IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval = 100, .expected{Sample{.timestamp = 100, .value = 1.0}}},
        IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 300, .value = 1.0}}, .interval = 400, .expected{Sample{.timestamp = 300, .value = 1.0}}}));
INSTANTIATE_TEST_SUITE_P(ManySamples,
                         IntervalDecodeIteratorFixture,
                         testing::Values(IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                        Sample{.timestamp = 200, .value = 1.0},
                                                                        Sample{.timestamp = 300, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                        Sample{.timestamp = 200, .value = 1.0},
                                                                        Sample{.timestamp = 300, .value = 1.0},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                        Sample{.timestamp = 150, .value = 1.0},
                                                                        Sample{.timestamp = 200, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                        Sample{.timestamp = 200, .value = 1.0},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 123, .value = 1.0},
                                                                        Sample{.timestamp = 152, .value = 1.0},
                                                                        Sample{.timestamp = 180, .value = 1.0},
                                                                        Sample{.timestamp = 215, .value = 1.0},
                                                                        Sample{.timestamp = 242, .value = 1.0},
                                                                        Sample{.timestamp = 275, .value = 1.0},
                                                                        Sample{.timestamp = 303, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 180, .value = 1.0},
                                                                        Sample{.timestamp = 275, .value = 1.0},
                                                                        Sample{.timestamp = 303, .value = 1.0},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 180, .value = 1.0},
                                                                        Sample{.timestamp = 275, .value = 1.0},
                                                                        Sample{.timestamp = 503, .value = 1.0},
                                                                        Sample{.timestamp = 603, .value = 1.0},
                                                                        Sample{.timestamp = 604, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 180, .value = 1.0},
                                                                        Sample{.timestamp = 275, .value = 1.0},
                                                                        Sample{.timestamp = 503, .value = 1.0},
                                                                        Sample{.timestamp = 604, .value = 1.0},
                                                                    }}));
INSTANTIATE_TEST_SUITE_P(StaleNan,
                         IntervalDecodeIteratorFixture,
                         testing::Values(IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}},
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = STALE_NAN},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 99, .value = 1.0},
                                                                        Sample{.timestamp = 100, .value = STALE_NAN},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = STALE_NAN},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 98, .value = 1.0},
                                                                        Sample{.timestamp = 99, .value = STALE_NAN},
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                    }},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 100, .value = STALE_NAN},
                                                                        Sample{.timestamp = 101, .value = 1.0},
                                                                        Sample{.timestamp = 200, .value = STALE_NAN},
                                                                        Sample{.timestamp = 201, .value = 1.0},
                                                                        Sample{.timestamp = 300, .value = STALE_NAN},
                                                                        Sample{.timestamp = 400, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .expected{
                                                                        Sample{.timestamp = 100, .value = STALE_NAN},
                                                                        Sample{.timestamp = 200, .value = STALE_NAN},
                                                                        Sample{.timestamp = 300, .value = STALE_NAN},
                                                                        Sample{.timestamp = 400, .value = 1.0},
                                                                    }}));
INSTANTIATE_TEST_SUITE_P(NoDownsampling,
                         IntervalDecodeIteratorFixture,
                         testing::Values(IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 98, .value = 1.0},
                                                                        Sample{.timestamp = 99, .value = STALE_NAN},
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                    },
                                                                    .interval = 0,
                                                                    .expected{
                                                                        Sample{.timestamp = 98, .value = 1.0},
                                                                        Sample{.timestamp = 99, .value = STALE_NAN},
                                                                        Sample{.timestamp = 100, .value = 1.0},
                                                                    }}));

}  // namespace