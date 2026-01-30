#include <gtest/gtest.h>

#include "series_data/decoder.h"
#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
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
using series_data::decoder::decorator::DownsamplingDecodeIterator;
using series_data::encoder::Sample;

struct DownsamplingDecodeIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::Timestamp interval;
  std::vector<Sample> expected{};
};

class DownsamplingDecodeIteratorFixture : public ::testing::TestWithParam<DownsamplingDecodeIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }
};

TEST_P(DownsamplingDecodeIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    std::ranges::copy(DownsamplingDecodeIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval),
                      DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

TEST_P(DownsamplingDecodeIteratorFixture, TestReset) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
    DownsamplingDecodeIterator iterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().interval);
    std::advance(iterator, GetParam().samples.size());

    iterator = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
    std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  });

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(
    OneSample,
    DownsamplingDecodeIteratorFixture,
    testing::Values(
        DownsamplingDecodeIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval = 100, .expected{Sample{.timestamp = 100, .value = 1.0}}},
        DownsamplingDecodeIteratorCase{.samples{Sample{.timestamp = 300, .value = 1.0}}, .interval = 400, .expected{Sample{.timestamp = 300, .value = 1.0}}}));
INSTANTIATE_TEST_SUITE_P(ManySamples,
                         DownsamplingDecodeIteratorFixture,
                         testing::Values(DownsamplingDecodeIteratorCase{.samples{
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
                                         DownsamplingDecodeIteratorCase{.samples{
                                                                            Sample{.timestamp = 100, .value = 1.0},
                                                                            Sample{.timestamp = 150, .value = 1.0},
                                                                            Sample{.timestamp = 200, .value = 1.0},
                                                                        },
                                                                        .interval = 100,
                                                                        .expected{
                                                                            Sample{.timestamp = 100, .value = 1.0},
                                                                            Sample{.timestamp = 200, .value = 1.0},
                                                                        }},
                                         DownsamplingDecodeIteratorCase{.samples{
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
                                         DownsamplingDecodeIteratorCase{.samples{
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
                         DownsamplingDecodeIteratorFixture,
                         testing::Values(DownsamplingDecodeIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN}},
                                                                        .interval = 100,
                                                                        .expected{
                                                                            Sample{.timestamp = 100, .value = STALE_NAN},
                                                                        }},
                                         DownsamplingDecodeIteratorCase{.samples{
                                                                            Sample{.timestamp = 99, .value = 1.0},
                                                                            Sample{.timestamp = 100, .value = STALE_NAN},
                                                                        },
                                                                        .interval = 100,
                                                                        .expected{
                                                                            Sample{.timestamp = 100, .value = STALE_NAN},
                                                                        }},
                                         DownsamplingDecodeIteratorCase{.samples{
                                                                            Sample{.timestamp = 98, .value = 1.0},
                                                                            Sample{.timestamp = 99, .value = STALE_NAN},
                                                                            Sample{.timestamp = 100, .value = 1.0},
                                                                        },
                                                                        .interval = 100,
                                                                        .expected{
                                                                            Sample{.timestamp = 100, .value = 1.0},
                                                                        }},
                                         DownsamplingDecodeIteratorCase{.samples{
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

}  // namespace