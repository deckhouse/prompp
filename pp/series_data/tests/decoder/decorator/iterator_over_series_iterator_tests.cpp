#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/decorator/changes_iterator.h"
#include "series_data/decoder/decorator/rate_iterator.h"
#include "series_data/decoder/decorator/resets_iterator.h"
#include "series_data/encoder.h"
#include "series_data/serialization/deserializer.h"
#include "series_data/serialization/serialized_data.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::TimeInterval;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::decorator::ChangesIterator;
using series_data::decoder::decorator::RateIterator;
using series_data::decoder::decorator::ResetsIterator;
using series_data::encoder::Sample;
using series_data::serialization::DataSerializer;
using series_data::serialization::SerializedDataView;

class IteratorOverSeriesIteratorFixture : public ::testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
};

TEST_F(IteratorOverSeriesIteratorFixture, RateIteratorWith2Chunks) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 2.0);
  encoder_.encode(0, 102, 3.0);
  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 0.0);

  const auto serialized_data = DataSerializer{storage_}.serialize();
  SerializedDataView serialized_view(serialized_data);

  auto [series_id, chunk_id] = serialized_view.next_series();

  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(RateIterator(serialized_view.create_series_iterator(chunk_id), TimeInterval{.min = 100, .max = 103}), DecodeIteratorSentinel{},
                    std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ((std::vector{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 102, .value = 3.0}, Sample{.timestamp = 103, .value = 0.0}}),
            actual_samples);
}

TEST_F(IteratorOverSeriesIteratorFixture, ChangesIteratorWith2Chunks) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 102, 1.0);
  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 0.0);

  const auto serialized_data = DataSerializer{storage_}.serialize();
  SerializedDataView serialized_view(serialized_data);

  auto [series_id, chunk_id] = serialized_view.next_series();

  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(ChangesIterator(serialized_view.create_series_iterator(chunk_id), TimeInterval{.min = 100, .max = 103}), DecodeIteratorSentinel{},
                    std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ((std::vector{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 103, .value = 0.0}}), actual_samples);
}

TEST_F(IteratorOverSeriesIteratorFixture, ResetsIteratorWith2Chunks) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 102, 1.0);
  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 0.0);

  const auto serialized_data = DataSerializer{storage_}.serialize();
  SerializedDataView serialized_view(serialized_data);

  auto [series_id, chunk_id] = serialized_view.next_series();

  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(ResetsIterator(serialized_view.create_series_iterator(chunk_id), TimeInterval{.min = 100, .max = 103}), DecodeIteratorSentinel{},
                    std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ((std::vector{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 103, .value = 0.0}}), actual_samples);
}

}  // namespace