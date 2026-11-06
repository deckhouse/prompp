#include <gtest/gtest.h>

#include "non_empty_quants.h"
#include "series_data/chunk_finalizer.h"
#include "series_data/encoder.h"
#include "series_data/unloading/unloader.h"

namespace {

using head::get_non_empty_quants;
using DataStorage = series_data::DataStorage<>;
using series_data::Encoder;
using series_data::unloading::Unloader;

constexpr int64_t kQuantDuration = 100;

class NonEmptyQuantsFixture : public ::testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};

  [[nodiscard]] std::vector<int64_t> non_empty_quants() const {
    const auto quants = get_non_empty_quants(storage_, kQuantDuration);
    return {quants.begin(), quants.end()};
  }
};

TEST_F(NonEmptyQuantsFixture, EmptyStorage) {
  // Arrange

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_TRUE(quants.empty());
}

TEST_F(NonEmptyQuantsFixture, ChunkInOneQuant) {
  // Arrange
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 199, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{100}), quants);
}

TEST_F(NonEmptyQuantsFixture, OpenChunkWithGap) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 100'000, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{0, 100'000}), quants);
}

TEST_F(NonEmptyQuantsFixture, FinalizedChunkWithGap) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 100'000, 1.0);
  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 200'000, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{0, 100'000, 200'000}), quants);
}

TEST_F(NonEmptyQuantsFixture, QuantsOfSeveralSeriesAreSortedAndUnique) {
  // Arrange
  encoder_.encode(0, 501, 1.0);
  encoder_.encode(1, 1, 1.0);
  encoder_.encode(2, 502, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{0, 500}), quants);
}

TEST_F(NonEmptyQuantsFixture, NegativeTimestamp) {
  // Arrange
  encoder_.encode(0, -1, 1.0);
  encoder_.encode(0, 0, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{-100, 0}), quants);
}

TEST_F(NonEmptyQuantsFixture, UnloadedSeriesWithOutdatedSample) {
  // Arrange
  encoder_.encode(0, 1'001, 1.0);
  encoder_.encode(0, 1'002, 1.0);
  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 1'003, 1.0);

  BareBones::ShrinkedToFitOStringStream snapshot;
  Unloader<> unloader{storage_};
  unloader.create_snapshot(snapshot);
  unloader.unload();
  encoder_.encode(0, 1, 1.0);

  // Act
  const auto quants = non_empty_quants();

  // Assert
  EXPECT_EQ((std::vector<int64_t>{0, 1'000}), quants);
}

}  // namespace
