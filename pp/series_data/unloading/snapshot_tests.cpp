#include <gtest/gtest.h>

#include <vector>

#include "bare_bones/streams.h"
#include "series_data/unloading/snapshot.h"

namespace {

using series_data::unloading::SnapshotChunkView;
using series_data::unloading::SnapshotReader;
using series_data::unloading::SnapshotWriter;

class SnapshotTestFixture : public testing::Test {
 protected:
  struct DecodedChunk {
    uint32_t ls_id;
    uint8_t chunk_id;
    std::vector<uint8_t> bytes;

    bool operator==(const DecodedChunk&) const = default;
  };

  template <class Chunks>
  void write(const Chunks& chunks) {
    SnapshotWriter::write_to(stream_, chunks);
  }

  void read() {
    SnapshotReader::visit(stream_.span<uint8_t>(), [this](const SnapshotChunkView& chunk) {
      decoded_.emplace_back(DecodedChunk{chunk.ls_id(), chunk.chunk_id(), std::vector<uint8_t>{chunk.bytes().begin(), chunk.bytes().end()}});
    });
  }

  BareBones::ShrinkedToFitOStringStream stream_;
  std::vector<DecodedChunk> decoded_;
};

TEST_F(SnapshotTestFixture, RoundTripPreservesChunkMetadataAndPayload) {
  // Arrange
  const std::array<uint8_t, 2> first_bytes{1, 2};
  const std::array<uint8_t, 3> second_bytes{3, 4, 5};
  const std::array<SnapshotChunkView, 2> chunks{{{3, 1, first_bytes}, {10, 2, second_bytes}}};

  const std::vector<DecodedChunk> expected{{3, 1, {1, 2}}, {10, 2, {3, 4, 5}}};

  // Act
  write(chunks);
  read();

  // Assert
  EXPECT_EQ(expected, decoded_);
}

TEST_F(SnapshotTestFixture, RoundTripPreservesEmptyChunkPayload) {
  // Arrange
  const std::array<SnapshotChunkView, 1> chunks{{{7, 3, {}}}};
  const std::vector<DecodedChunk> expected{{7, 3, {}}};

  // Act
  write(chunks);
  read();

  // Assert
  EXPECT_EQ(expected, decoded_);
}

TEST_F(SnapshotTestFixture, RoundTripPreservesRepeatedAndChangedChunkIds) {
  // Arrange
  const std::array<uint8_t, 1> first_bytes{1};
  const std::array<uint8_t, 2> second_bytes{2, 3};
  const std::array<uint8_t, 3> third_bytes{4, 5, 6};
  const std::array<SnapshotChunkView, 3> chunks{{{1, 4, first_bytes}, {2, 4, second_bytes}, {5, 9, third_bytes}}};
  const std::vector<DecodedChunk> expected{{1, 4, {1}}, {2, 4, {2, 3}}, {5, 9, {4, 5, 6}}};

  // Act
  write(chunks);
  read();

  // Assert
  EXPECT_EQ(expected, decoded_);
}

TEST_F(SnapshotTestFixture, EmptySnapshotHasNoChunks) {
  // Arrange
  const std::array<SnapshotChunkView, 0> chunks{};

  // Act
  write(chunks);
  read();

  // Assert
  EXPECT_TRUE(decoded_.empty());
}

}  // namespace
