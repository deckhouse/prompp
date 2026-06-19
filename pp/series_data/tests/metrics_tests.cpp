#include <gtest/gtest.h>

#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::EncodingType;
using series_data::OutdatedChunkMerger;
using series_data::chunk::FinalizedChunkList;

template <uint8_t kSamplesPerChunk = series_data::kSamplesPerChunkDefault>
class DataStorageMetricsTestTrait {
 protected:
  DataStorage storage_;
  Encoder<kSamplesPerChunk> encoder_{storage_};

  [[nodiscard]] double chunk_count(EncodingType encoding_type) const noexcept { return storage_.metrics->get_chunk_count(encoding_type); }

  [[nodiscard]] double finalized_chunks_count() const noexcept { return storage_.metrics->finalized_chunks().value(); }

  [[nodiscard]] double outdated_samples_count() const { return storage_.metrics->outdated_samples().value(); }

  [[nodiscard]] double outdated_chunks_count() const { return storage_.metrics->outdated_chunks().value(); }

  [[nodiscard]] double timestamp_states_count() const {
    storage_.metrics->refresh_metrics();
    return storage_.metrics->timestamp_states_count();
  }
};

class DataStorageMetricsTestFixture : public DataStorageMetricsTestTrait<>, public testing::Test {};

TEST_F(DataStorageMetricsTestFixture, InitialMetricsAreZero) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0, outdated_samples_count());
  EXPECT_EQ(0, outdated_chunks_count());
  EXPECT_EQ(0, finalized_chunks_count());
  EXPECT_EQ(0, timestamp_states_count());
  EXPECT_EQ(0, chunk_count(EncodingType::kUint32Constant));
  EXPECT_EQ(0, chunk_count(EncodingType::kFloat32Constant));
  EXPECT_EQ(0, chunk_count(EncodingType::kDoubleConstant));
  EXPECT_EQ(0, chunk_count(EncodingType::kTwoDoubleConstant));
  EXPECT_EQ(0, chunk_count(EncodingType::kAscInteger));
  EXPECT_EQ(0, chunk_count(EncodingType::kAscIntegerThenValuesGorilla));
  EXPECT_EQ(0, chunk_count(EncodingType::kValuesGorilla));
  EXPECT_EQ(0, chunk_count(EncodingType::kGorilla));
}

TEST_F(DataStorageMetricsTestFixture, Uint32ConstantChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);

  // Assert
  EXPECT_EQ(1, chunk_count(EncodingType::kUint32Constant));
}

TEST_F(DataStorageMetricsTestFixture, SwitchToTwoDoubleConstantUpdatesChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 3, -1.1);
  encoder_.encode(0, 4, -1.1);

  // Assert
  EXPECT_EQ(0, chunk_count(EncodingType::kFloat32Constant));
  EXPECT_EQ(1, chunk_count(EncodingType::kTwoDoubleConstant));
}

TEST_F(DataStorageMetricsTestFixture, SwitchFromUint32ConstantToAscIntegerUpdatesChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 2.0);

  // Assert
  EXPECT_EQ(0, chunk_count(EncodingType::kUint32Constant));
  EXPECT_EQ(1, chunk_count(EncodingType::kAscInteger));
}

TEST_F(DataStorageMetricsTestFixture, SwitchToAscIntegerThenValuesGorillaUpdatesChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 2.1);
  encoder_.encode(0, 6, 2.2);

  // Assert
  EXPECT_EQ(0, chunk_count(EncodingType::kAscInteger));
  EXPECT_EQ(1, chunk_count(EncodingType::kAscIntegerThenValuesGorilla));
}

TEST_F(DataStorageMetricsTestFixture, DoubleConstantChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);

  // Assert
  EXPECT_EQ(1, chunk_count(EncodingType::kDoubleConstant));
}

TEST_F(DataStorageMetricsTestFixture, SwitchToValuesGorillaUpdatesChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  // Assert
  EXPECT_EQ(1, chunk_count(EncodingType::kTwoDoubleConstant));
  EXPECT_EQ(1, chunk_count(EncodingType::kValuesGorilla));
}

TEST_F(DataStorageMetricsTestFixture, SwitchToGorillaUpdatesChunkCount) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.0);
  encoder_.encode(0, 4, 3.0);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  EXPECT_EQ(0, chunk_count(EncodingType::kTwoDoubleConstant));
  EXPECT_EQ(1, chunk_count(EncodingType::kGorilla));
}

TEST_F(DataStorageMetricsTestFixture, OutdatedSamplesAndChunksCounters) {
  // Arrange
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 2.0);

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(1, 1, 2.0);

  // Assert
  EXPECT_EQ(3, outdated_samples_count());
  EXPECT_EQ(2, outdated_chunks_count());
}

TEST_F(DataStorageMetricsTestFixture, TimestampStatesCountReflectsEncoderState) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  // Assert
  EXPECT_EQ(storage_.timestamp_encoder.states_count(), timestamp_states_count());
}

class DataStorageMetricsFinalizeTestFixture : public DataStorageMetricsTestTrait<3>, public testing::Test {};

TEST_F(DataStorageMetricsFinalizeTestFixture, FinalizeIncrementsFinalizedChunksCount) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);

  // Act
  encoder_.encode(0, 4, 1.0);

  // Arrange
  EXPECT_EQ(1, finalized_chunks_count());
  EXPECT_EQ(2, chunk_count(EncodingType::kUint32Constant));
}

template <uint8_t kSamplesPerChunk = series_data::kSamplesPerChunkDefault>
class DataStorageMetricsMergeOutdatedChunksTestTrait : public DataStorageMetricsTestTrait<kSamplesPerChunk> {
 protected:
  OutdatedChunkMerger<decltype(DataStorageMetricsMergeOutdatedChunksTestTrait::encoder_)> merger_{DataStorageMetricsMergeOutdatedChunksTestTrait::encoder_};
};

class DataStorageMetricsMergeOutdatedChunksTestFixture : public DataStorageMetricsMergeOutdatedChunksTestTrait<>, public testing::Test {};

TEST_F(DataStorageMetricsMergeOutdatedChunksTestFixture, MergeOpenChunkPreservesUint32ConstantChunkCount) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 0, 1.0);

  // Act
  merger_.merge();

  // Assert
  EXPECT_EQ(3, outdated_samples_count());
  EXPECT_EQ(1, outdated_chunks_count());
  EXPECT_EQ(1, chunk_count(EncodingType::kUint32Constant));
}

class DataStorageMetricsMergeFinalizedTestFixture : public DataStorageMetricsMergeOutdatedChunksTestTrait<4>, public testing::Test {};

TEST_F(DataStorageMetricsMergeFinalizedTestFixture, MergeFinalizedChunkPreservesChunkAndFinalizedCounts) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, 1.0);
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Act
  merger_.merge();

  // Assert
  EXPECT_EQ(1, outdated_samples_count());
  EXPECT_EQ(1, outdated_chunks_count());
  EXPECT_EQ(1, finalized_chunks_count());
  EXPECT_EQ(2, chunk_count(EncodingType::kUint32Constant));
}

}  // namespace
