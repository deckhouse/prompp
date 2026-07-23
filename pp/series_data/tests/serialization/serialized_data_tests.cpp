#include <gtest/gtest.h>

#include "series_data/chunk_finalizer.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/serialization/serialized_data.h"

namespace {

using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::serialization::DataSerializer;
using series_data::serialization::SerializedData;
using series_data::serialization::SerializedDataView;

class SerializedDataViewEnumerateSeriesFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  DataSerializer serializer_{storage_};

  struct ChunkInfo {
    uint32_t series_id;
    uint32_t chunk_id;

    bool operator==(const ChunkInfo& other) const noexcept = default;
  };

  [[nodiscard]] SerializedData serialize() noexcept {
    auto data = serializer_.serialize();
    storage_.reset();
    return data;
  }

  std::vector<ChunkInfo> enumerate_series() noexcept {
    std::vector<ChunkInfo> series_ids;
    SerializedDataView{serialize()}.enumerate_series([&](const auto& chunk, uint32_t chunk_id) { series_ids.emplace_back(chunk.label_set_id, chunk_id); });
    return series_ids;
  }
};

TEST_F(SerializedDataViewEnumerateSeriesFixture, NoSeries) {
  // Arrange

  // Act
  const auto series_ids = enumerate_series();

  // Assert
  EXPECT_TRUE(series_ids.empty());
}

TEST_F(SerializedDataViewEnumerateSeriesFixture, OneSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Act
  const auto series_ids = enumerate_series();

  // Assert
  EXPECT_EQ((std::vector{ChunkInfo{.series_id = 0U, .chunk_id = 0}}), series_ids);
}

TEST_F(SerializedDataViewEnumerateSeriesFixture, TwoSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 2.0);

  // Act
  const auto series_ids = enumerate_series();

  // Assert
  EXPECT_EQ((std::vector{ChunkInfo{.series_id = 0U, .chunk_id = 0}, ChunkInfo{.series_id = 1U, .chunk_id = 1U}}), series_ids);
}

TEST_F(SerializedDataViewEnumerateSeriesFixture, OneSeriesWithTwoChunks) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 4, 1.0);

  // Act
  const auto series_ids = enumerate_series();

  // Assert
  EXPECT_EQ((std::vector{ChunkInfo{.series_id = 0U, .chunk_id = 0}}), series_ids);
}

TEST_F(SerializedDataViewEnumerateSeriesFixture, TwoSeriesWithMultipleChunksEach) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 2.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 2.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 2.0);

  // Act
  const auto series_ids = enumerate_series();

  // Assert
  EXPECT_EQ((std::vector{ChunkInfo{.series_id = 0U, .chunk_id = 0}, ChunkInfo{.series_id = 1U, .chunk_id = 2U}}), series_ids);
}

}  // namespace
