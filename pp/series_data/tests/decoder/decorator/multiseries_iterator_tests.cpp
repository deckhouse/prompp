#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/multiseries_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::MultiSeriesIterator;
using series_data::encoder::Sample;

using MultiSeriesMinIterator = MultiSeriesIterator<UniversalDecodeIterator, series_data::decoder::decorator::FindMinElement>;

class MultiSeriesIteratorMinElementFixture : public ::testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::Vector<UniversalDecodeIterator> iterators_;
  BareBones::Vector<Sample> samples_;

  void create_iterator(uint32_t series_id) {
    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[series_id], [this]<typename Iterator>(Iterator&& begin, auto&&) {
      iterators_.push_back(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)});
    });
  }

  void get_samples() { std::ranges::copy(MultiSeriesMinIterator{std::move(iterators_)}, DecodeIteratorSentinel{}, std::back_insert_iterator(samples_)); }
};

TEST_F(MultiSeriesIteratorMinElementFixture, EmptyIteratorListIsImmediatelyExhausted) {
  // Arrange
  const MultiSeriesMinIterator it(std::move(iterators_));

  // Act

  // Assert
  EXPECT_EQ(it, DecodeIteratorSentinel{});
}

TEST_F(MultiSeriesIteratorMinElementFixture, SingleSeriesOneSampleYieldsThatSample) {
  // Arrange
  encoder_.encode(0, 100, 3.5);

  create_iterator(0);

  // Act
  get_samples();

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 100, .value = 3.5}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, MinValueAcrossTwoSeries) {
  // Arrange
  encoder_.encode(0, 10, 7.0);
  encoder_.encode(1, 20, 2.0);

  create_iterator(0);
  create_iterator(1);

  // Act
  get_samples();

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 20, .value = 2.0}}), samples_);
}

TEST_F(MultiSeriesIteratorMinElementFixture, EqualValuesKeepsFirstSeenTimestamp) {
  // Arrange
  encoder_.encode(0, 100, 5.0);
  encoder_.encode(1, 200, 5.0);

  create_iterator(0);
  create_iterator(1);

  // Act
  get_samples();

  // Assert
  EXPECT_EQ((BareBones::Vector{Sample{.timestamp = 100, .value = 5.0}}), samples_);
}

}  // namespace
