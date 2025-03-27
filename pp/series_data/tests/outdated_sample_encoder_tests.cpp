#include <gmock/gmock.h>

#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace {

using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::encoder::Sample;

class OutdatedSampleEncoderFixture : public testing::Test {
 protected:
  using ExpectedSampleList = BareBones::Vector<Sample>;

  DataStorage storage_;
  Encoder<3> encoder_{storage_};
};

TEST_F(OutdatedSampleEncoderFixture, NoMergeAtEncode) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);

  // Act
  encoder_.encode(0, 0, 1.0);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  ASSERT_TRUE(storage_.outdated_chunks.contains(0));
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
      },
      Decoder::decode_outdated_chunk(storage_.outdated_chunks.find(0)->second)));
}

TEST_F(OutdatedSampleEncoderFixture, MergeAtEncode) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);

  // Act
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Assert
  EXPECT_TRUE(std::ranges::equal(ExpectedSampleList{{.timestamp = 5, .value = 1.0}, {.timestamp = 6, .value = 1.0}},
                                 Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  EXPECT_TRUE(std::ranges::equal(ExpectedSampleList{{.timestamp = 0, .value = 1.0}, {.timestamp = 1, .value = 1.0}, {.timestamp = 2, .value = 1.0}},
                                 Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.find(0)->second.front())));
  ASSERT_FALSE(storage_.outdated_chunks.contains(0));
}

TEST_F(OutdatedSampleEncoderFixture, MergeOneChunk) {
  // Arrange
  encoder_.encode(0, 5, 1.0);
  encoder_.encode(0, 6, 1.0);
  encoder_.encode(0, 0, 1.0);

  // Act
  OutdatedSampleEncoder<>::merge_outdated_chunks(encoder_);
  // outdated_sample_encoder_.merge_outdated_chunks(encoder_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      ExpectedSampleList{
          {.timestamp = 0, .value = 1.0},
          {.timestamp = 5, .value = 1.0},
          {.timestamp = 6, .value = 1.0},
      },
      Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0])));
  ASSERT_FALSE(storage_.outdated_chunks.contains(0));
}

}  // namespace