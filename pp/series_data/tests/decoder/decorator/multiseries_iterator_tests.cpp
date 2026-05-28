#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/last_over_time.h"
#include "series_data/decoder/decorator/lookback_delta_iterator.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/multiseries_iterator.h"
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
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::LastOverTimeWithStaleNansIterator;
using series_data::decoder::decorator::LookbackDeltaIterator;
using series_data::decoder::decorator::MultiSeriesIterator;
using series_data::decoder::decorator::StepLookbackDeltaWindowCalculator;
using series_data::decoder::decorator::WindowFunctionParameters;
using series_data::encoder::Sample;

using MultiSeriesMinIterator = MultiSeriesIterator<LookbackDeltaIterator<LastOverTimeWithStaleNansIterator<>>,
                                                   series_data::decoder::decorator::FindMinElement,
                                                   StepLookbackDeltaWindowCalculator>;

class MultiSeriesIteratorFixture : public ::testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::Vector<LookbackDeltaIterator<LastOverTimeWithStaleNansIterator<>>> iterators_;
  BareBones::Vector<Sample> samples_;

  void create_iterators(std::initializer_list<uint32_t> series_ids, const WindowFunctionParameters& parameters) {
    const auto initial_window = StepLookbackDeltaWindowCalculator::initial_window(parameters);

    for (const auto series_id : series_ids) {
      Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[series_id], [&]<typename Iterator>(Iterator&& begin, auto&&) {
        iterators_.emplace_back(
            LastOverTimeWithStaleNansIterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, initial_window),
            parameters.lookback_delta);
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

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 99, .max = 100},
      .step = 100,
      .lookback_delta = 1,
  };
  create_iterators({0}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 100, .value = 3.5}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, MinValueAcrossTwoSeries) {
  // Arrange
  encoder_.encode(0, 11, 7.0);
  encoder_.encode(1, 20, 2.0);

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 10, .max = 20},
      .step = 10,
      .lookback_delta = 10,
  };
  create_iterators({0, 1}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 20, .value = 2.0}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, TwoWindowsAcrossTwoSeries) {
  // Arrange
  encoder_.encode(0, 150, 5.0);
  encoder_.encode(0, 201, 6.0);
  encoder_.encode(1, 150, 4.0);
  encoder_.encode(1, 201, 5.0);

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 149, .max = 250},
      .step = 50,
      .lookback_delta = 1,
  };
  create_iterators({0, 1}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 150, .value = 4.0},
                Sample{.timestamp = 200, .value = STALE_NAN},
                Sample{.timestamp = 250, .value = 5.0},
            }),
            samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, LastStaleNan) {
  // Arrange
  encoder_.encode(0, 101, 5.0);
  encoder_.encode(0, 200, 6.0);
  encoder_.encode(1, 101, 4.0);
  encoder_.encode(1, 200, 5.0);

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 100, .max = 250},
      .step = 50,
      .lookback_delta = 50,
  };
  create_iterators({0, 1}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 150, .value = 4.0},
                Sample{.timestamp = 200, .value = 5.0},
                Sample{.timestamp = 250, .value = STALE_NAN},
            }),
            samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, LoopbackInterval) {
  // Arrange
  encoder_.encode(0, 50, 20.0);
  encoder_.encode(1, 80, 10.0);
  encoder_.encode(0, 150, 20.0);
  encoder_.encode(1, 180, 30.0);

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 0, .max = 200},
      .step = 100,
      .lookback_delta = 100,
  };
  create_iterators({0, 1}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 100, .value = 10.0},
                Sample{.timestamp = 200, .value = 20.0},
            }),
            samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, StaleNansInSeries) {
  // Arrange
  encoder_.encode(0, 50, 20.0);
  encoder_.encode(1, 80, 10.0);
  encoder_.encode(0, 150, STALE_NAN);
  encoder_.encode(1, 180, STALE_NAN);
  encoder_.encode(0, 250, 20.0);
  encoder_.encode(1, 280, 30.0);

  constexpr WindowFunctionParameters parameters = {
      .interval = TimeInterval{.min = 0, .max = 300},
      .step = 100,
      .lookback_delta = 100,
  };
  create_iterators({0, 1}, parameters);

  // Act
  get_samples(parameters);

  // Assert
  EXPECT_EQ((BareBones::Vector{
                Sample{.timestamp = 100, .value = 10.0},
                Sample{.timestamp = 200, .value = STALE_NAN},
                Sample{.timestamp = 300, .value = 20.0},
            }),
            samples_);
}

}  // namespace
