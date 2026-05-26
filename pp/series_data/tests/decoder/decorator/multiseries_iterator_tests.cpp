#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/last_over_step.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/multiseries_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::TimeInterval;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::LastOverStepWithStaleNansIterator;
using series_data::decoder::decorator::MultiSeriesIterator;
using series_data::decoder::decorator::StepLookbackDeltaWindowCalculator;
using series_data::decoder::decorator::WindowFunctionParameters;
using series_data::encoder::Sample;

using MultiSeriesMinIterator =
    MultiSeriesIterator<LastOverStepWithStaleNansIterator<>, series_data::decoder::decorator::FindMinElement, StepLookbackDeltaWindowCalculator>;

class MultiSeriesIteratorFixture : public ::testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::Vector<LastOverStepWithStaleNansIterator<>> iterators_;
  BareBones::Vector<Sample> samples_;

  void create_iterators(std::initializer_list<uint32_t> series_ids, TimeInterval initial_interval) {
    for (const auto series_id : series_ids) {
      Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[series_id], [&]<typename Iterator>(Iterator&& begin, auto&&) {
        iterators_.emplace_back(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, initial_interval);
      });
    }
  }
};

class MultiSeriesIteratorMinElementFixture : public MultiSeriesIteratorFixture {
 protected:
  void get_samples(const WindowFunctionParameters& parameters) {
    std::ranges::copy(MultiSeriesMinIterator{std::move(iterators_), parameters}, DecodeIteratorSentinel{}, std::back_insert_iterator(samples_));
  }
};

TEST_F(MultiSeriesIteratorMinElementFixture, EmptyIteratorListIsImmediatelyExhausted) {
  // Arrange

  // Act
  get_samples({});

  // Assert
  EXPECT_TRUE(samples_.empty());
}

TEST_F(MultiSeriesIteratorMinElementFixture, SingleSeriesOneSampleYieldsThatSample) {
  // Arrange
  encoder_.encode(0, 100, 3.5);

  create_iterators({0}, {.min = 100, .max = 100});

  // Act
  get_samples({.interval = TimeInterval{.min = 100, .max = 100}, .step = 100});

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 100, .value = 3.5}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, MinValueAcrossTwoSeries) {
  // Arrange
  encoder_.encode(0, 10, 7.0);
  encoder_.encode(1, 20, 2.0);

  create_iterators({0, 1}, {.min = 10, .max = 20});

  // Act
  get_samples({.interval = TimeInterval{.min = 100, .max = 100}, .step = 100});

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 20, .value = 2.0}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, TwoWindowsAcrossTwoSeries) {
  // Arrange
  encoder_.encode(0, 100, 5.0);
  encoder_.encode(0, 201, 6.0);
  encoder_.encode(1, 100, 4.0);
  encoder_.encode(1, 201, 5.0);

  create_iterators({0, 1}, {.min = 100, .max = 150});

  // Act
  get_samples({.interval = TimeInterval{.min = 100, .max = 250}, .step = 50});

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 150, .value = 4.0},
                Sample{.timestamp = 200, .value = STALE_NAN},
                Sample{.timestamp = 250, .value = 5.0},
            }),
            samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, SkipLastStaleNan) {
  // Arrange
  encoder_.encode(0, 100, 5.0);
  encoder_.encode(0, 200, 6.0);
  encoder_.encode(1, 100, 4.0);
  encoder_.encode(1, 200, 5.0);

  create_iterators({0, 1}, {.min = 100, .max = 150});

  // Act
  get_samples({.interval = TimeInterval{.min = 100, .max = 250}, .step = 50});

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 150, .value = 4.0},
                Sample{.timestamp = 200, .value = 5.0},
            }),
            samples_);
}

}  // namespace
