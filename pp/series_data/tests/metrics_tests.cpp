#include <gtest/gtest.h>

#include <bit>
#include <string>
#include <string_view>

#include "metrics/storage.h"
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

  // The gauge is pushed on state creation, so it always reflects the encoder without an explicit refresh.
  [[nodiscard]] double timestamp_states_count() const { return storage_.metrics->timestamp_states_count(); }
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

// The timestamp_states gauge is pushed on state creation only (states_.size() grows there; erase merely marks a hole and
// does not change states_.size()). It must therefore stay equal to encoder.states_count() both while states are created
// and after a finalize erases states, without any scrape-time pull from the encoder.
TEST_F(DataStorageMetricsFinalizeTestFixture, TimestampStatesCountMatchesEncoderWhileCreatingStates) {
  // Arrange & Act: create timestamp states for two series.
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  // Assert: states were created and the pushed gauge matches the encoder.
  ASSERT_GT(storage_.timestamp_encoder.states_count(), 0u);
  EXPECT_EQ(storage_.timestamp_encoder.states_count(), timestamp_states_count());
}

TEST_F(DataStorageMetricsFinalizeTestFixture, TimestampStatesCountMatchesEncoderAfterFinalize) {
  // Arrange: fill the first chunk (kSamplesPerChunk == 3) for series 0.
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  // Act: the 4th sample finalizes the first chunk, erasing its timestamp states.
  encoder_.encode(0, 4, 4.0);

  // Assert: erase does not change states_.size(), so the create-only push still matches the encoder.
  EXPECT_EQ(storage_.timestamp_encoder.states_count(), timestamp_states_count());
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

[[nodiscard]] std::string read_address_label(const metrics::MetricsPageControlBlock& page) {
  for (const metrics::Metric* metric : page) {
    for (const auto* label_pair : metric->go_metric()->metric->labels) {
      if (static_cast<std::string_view>(*label_pair->name) == "address") {
        return std::string{static_cast<std::string_view>(*label_pair->value)};
      }
    }
  }
  return {};
}

// Regression test for a use-after-free where the "address" label value was a non-owning view into a string stored in the
// DataStorage. After the DataStorage was destroyed the metrics page was only detached (not yet removed), so a concurrent
// scrape still holding the page would read freed memory and emit an invalid (non-UTF-8) label value. The label string is
// now owned by the page, so it stays valid until the page itself is reclaimed by remove_unused_pages().
TEST(DataStorageMetricsLifetimeTest, AddressLabelOwnedByPageSurvivesStorageDestruction) {
  // Arrange
  auto* storage = new DataStorage();
  auto* page = storage->metrics;
  const auto expected = std::to_string(std::bit_cast<uint64_t>(storage));
  ASSERT_EQ(expected, read_address_label(*page));

  // Act: destroy the storage. The page is only detached, not physically removed yet.
  delete storage;

  // Assert: the page still owns a valid "address" string (no dangling view / use-after-free).
  EXPECT_EQ(expected, read_address_label(*page));

  // Cleanup: reclaim the detached page.
  metrics::storage.remove_unused_pages();
}

}  // namespace
