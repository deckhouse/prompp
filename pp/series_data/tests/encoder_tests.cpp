#include <gtest/gtest.h>

#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace {

using BareBones::BitSequenceReader;
using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::EncodingType;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::chunk::FinalizedChunkList;
using series_data::chunk::OutdatedChunk;
using series_data::encoder::BitSequenceWithItemsCount;
using series_data::encoder::GorillaEncoder;
using series_data::encoder::SampleList;
using series_data::encoder::timestamp::TimestampDecoder;
using series_data::encoder::value::TwoDoubleConstantEncoder;

template <uint8_t kSamplesPerChunkValue>
class EncoderTestTrait {
 protected:
  static constexpr auto kSamplesPerChunk = kSamplesPerChunkValue;

  using ListOfSampleList = BareBones::Vector<SampleList>;

  DataStorage storage_;
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder_{clock_};
  Encoder<decltype(outdated_sample_encoder_), kSamplesPerChunk> encoder_{storage_, outdated_sample_encoder_};

  [[nodiscard]] const DataChunk& chunk(uint32_t ls_id) const noexcept { return storage_.open_chunks[ls_id]; }
  [[nodiscard]] const FinalizedChunkList* finalized_chunks(uint32_t ls_id) const noexcept {
    if (auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return &it->second;
    }

    return nullptr;
  }
  [[nodiscard]] const OutdatedChunk* outdated_chunk(uint32_t ls_id) const noexcept {
    if (auto it = storage_.outdated_chunks.find(ls_id); it != storage_.outdated_chunks.end()) {
      return &it->second;
    }

    return nullptr;
  }

  [[nodiscard]] const BitSequenceWithItemsCount& open_chunk_timestamp(uint32_t ls_id) const noexcept {
    return storage_.timestamp_encoder.get_stream(storage_.open_chunks[ls_id].timestamp_encoder_state_id);
  }
  [[nodiscard]] BitSequenceReader open_chunk_timestamp_reader(uint32_t ls_id) const noexcept { return open_chunk_timestamp(ls_id).reader(); }

  [[nodiscard]] BareBones::Vector<int64_t> decode_open_chunk_timestamp_list(uint32_t ls_id) const noexcept {
    auto& timestamp_stream = open_chunk_timestamp(ls_id);
    return TimestampDecoder::decode_all(timestamp_stream.reader(), timestamp_stream.count());
  }
};

class EncodeTestFixture : public EncoderTestTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(EncodeTestFixture, EncodeUint32Constant) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(1.0, chunk(0).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeFloat32Constant) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 128.625);
  encoder_.encode(0, 2, 128.625);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(128.625, chunk(0).encoder.float32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeFloat32ConstantNegativeValue) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(-1.0, chunk(0).encoder.float32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToTwoDoubleEncoderFromFloat32ConstantEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 3, -1.1);
  encoder_.encode(0, 4, -1.1);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);

  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(-1.0, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(-1.1, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstant) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(1.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstantNegativeValue) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.1);
  encoder_.encode(0, 2, -1.1);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(-1.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToTwoDoubleEncoderFromUint32ConstantEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.1);
  encoder_.encode(0, 4, 1.1);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);

  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(1.0, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(1.1, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, SwitchToTwoDoubleEncoderFromDoubleConstantEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 1.2);
  encoder_.encode(0, 4, 1.2);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);

  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(1.1, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(1.2, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeTestFixture, AscIntegerValuesGorillaEncoderWith1Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, STALE_NAN},
                {4, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, AscIntegerValuesGorillaEncoderWith2Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, STALE_NAN},
                {4, 2.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, AscIntegerValuesGorillaEncoder4_4_1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, 1.0);
  encoder_.encode(0, 5, 2.0);
  encoder_.encode(0, 6, 2.0);
  encoder_.encode(0, 7, 2.0);
  encoder_.encode(0, 8, 2.0);
  encoder_.encode(0, 9, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, 1.0},
                {4, 1.0},
                {5, 2.0},
                {6, 2.0},
                {7, 2.0},
                {8, 2.0},
                {9, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, IntegerValuesGorillaEncoderWith3Value1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, 1.0},
                {4, STALE_NAN},
                {5, 2.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToDoubleConstantEncoderFromIntegerValuesGorillaEncoderWithUniqueTimeserie) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 2.1);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(2.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ(BareBones::Vector<int64_t>{5}, decode_open_chunk_timestamp_list(0));

  auto finalized = finalized_chunks(0);
  ASSERT_NE(finalized, nullptr);
  EXPECT_EQ(EncodingType::kAscIntegerValuesGorilla, finalized->front().encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, finalized->front()));
}

TEST_F(EncodeTestFixture, SwitchToDoubleConstantEncoderFromIntegerValuesGorillaEncoderWithNonUniqueTimeserie) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(1, 3, 1.0);

  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(1, 4, 1.0);

  encoder_.encode(0, 5, 2.1);
  encoder_.encode(1, 5, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(2.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ(BareBones::Vector<int64_t>{5}, decode_open_chunk_timestamp_list(0));

  ASSERT_EQ(EncodingType::kUint32Constant, chunk(1).encoding_state.encoding_type);
  EXPECT_EQ(1.0, chunk(1).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4, 5}), decode_open_chunk_timestamp_list(1));

  auto finalized = finalized_chunks(0);
  ASSERT_NE(finalized, nullptr);
  EXPECT_EQ(EncodingType::kAscIntegerValuesGorilla, finalized->front().encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, finalized->front()));
}

TEST_F(EncodeTestFixture, ValuesGorillaEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_TRUE(std::ranges::equal((SampleList{
                                     {.timestamp = 1, .value = 1.1},
                                     {.timestamp = 2, .value = 2.0},
                                     {.timestamp = 3, .value = 3.0},
                                     {.timestamp = 4, .value = 3.0},
                                 }),
                                 Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0))));
}

TEST_F(EncodeTestFixture, GorillaEncoder) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.0);
  encoder_.encode(0, 4, 3.0);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);

  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.1},
                        {.timestamp = 2, .value = 1.1},
                        {.timestamp = 3, .value = 2.0},
                        {.timestamp = 4, .value = 3.0},
                        {.timestamp = 5, .value = STALE_NAN}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeUint32ConstantWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(1.0, chunk(0).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeUint32ConstantWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(1.0, chunk(0).encoder.uint32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeFloat32ConstantWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(-1.0, chunk(0).encoder.float32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, -1.0},
                {2, -1.0},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeFloat32ConstantWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(-1.0, chunk(0).encoder.float32_constant.value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, -1.0},
                {2, -1.0},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstantWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(1.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 1.1},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeDoubleConstantWithS2talenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ(1.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 1.1},
                {3, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeTwoDoubleConstantWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(0, 4, 2.1);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(1.1, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(2.1, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4, 5}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 1.1},
                {3, 2.1},
                {4, 2.1},
                {5, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeTwoDoubleConstantWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(0, 4, 2.1);
  encoder_.encode(0, 5, STALE_NAN);
  encoder_.encode(0, 6, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(1.1, encoder.value1());
  EXPECT_EQ(2, encoder.value1_count());
  EXPECT_EQ(2.1, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4, 5}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 1.1},
                {3, 2.1},
                {4, 2.1},
                {5, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeAscValuesWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeAscValuesWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeValuesGorillaWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeValuesGorillaWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeGorillaWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);

  encoder_.encode(0, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeGorillaWith2Stalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);

  encoder_.encode(0, 2, 2.0);

  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToAscEncoderFromUint32WithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, STALE_NAN},
                {4, 2.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToAscEncoderFromFloat32WithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, -1.0},
                {2, -1.0},
                {3, STALE_NAN},
                {4, 2.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToAscEncoderFromTwoDoubleWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, STALE_NAN},
                {4, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToAscEncoderFromTwoDoubleWithStalenan4_4_1) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, 1.0);
  encoder_.encode(0, 5, 2.0);
  encoder_.encode(0, 6, 2.0);
  encoder_.encode(0, 7, 2.0);
  encoder_.encode(0, 8, 2.0);
  encoder_.encode(0, 9, STALE_NAN);
  encoder_.encode(0, 10, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 1.0},
                {3, 1.0},
                {4, 1.0},
                {5, 2.0},
                {6, 2.0},
                {7, 2.0},
                {8, 2.0},
                {9, STALE_NAN},
                {10, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToValuesGorillaEncoderFromUint32WithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 2.0);
  encoder_.encode(1, 1, 2.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(1, 3, 2.0);
  encoder_.encode(0, 4, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 2.0},
                {2, 2.0},
                {3, STALE_NAN},
                {4, 1.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToValuesGorillaFromFloat32WithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(1, 1, 1.0);
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(1, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(1, 3, 1.0);
  encoder_.encode(0, 4, -2.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, -1.0},
                {2, -1.0},
                {3, STALE_NAN},
                {4, -2.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, SwitchToValuesGorillaFromTwoDoubleWithStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 1.0);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(1, 3, 1.0);
  encoder_.encode(0, 4, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, STALE_NAN},
                {4, 1.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, AscIntegerValuesGorillaEncoderValueAfterStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 4.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
                {5, 4.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, ValuesGorillaEncoderValueAfterStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 1.1);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(1, 3, 1.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 4.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
                {5, 4.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, GorillaEncoderValueAfterStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 5, 4.0);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);
  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
                {5, 4.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeTwoStalenan) {
  // Arrange

  // Act
  encoder_.encode(0, 1, STALE_NAN);
  encoder_.encode(0, 2, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_TRUE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeStalenanAndValue) {
  // Arrange

  // Act
  encoder_.encode(0, 1, STALE_NAN);
  encoder_.encode(0, 2, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, STALE_NAN},
                {2, 1.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeUint32StalenanUint32) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, STALE_NAN);
  encoder_.encode(0, 3, 1.0);

  // Assert
  ASSERT_NE(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, STALE_NAN},
                {3, 1.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeFloat32StalenanFloat32) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 2, STALE_NAN);
  encoder_.encode(0, 3, -1.0);

  // Assert
  ASSERT_NE(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, -1.0},
                {2, STALE_NAN},
                {3, -1.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, EncodeDoubleStalenanDouble) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, STALE_NAN);
  encoder_.encode(0, 3, 1.1);

  // Assert
  ASSERT_NE(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, STALE_NAN},
                {3, 1.1},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeTestFixture, Encode2DoubleStalenan2Double) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.2);
  encoder_.encode(0, 3, STALE_NAN);
  encoder_.encode(0, 4, 1.2);

  // Assert
  ASSERT_NE(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  ASSERT_FALSE(chunk(0).encoding_state.has_last_stalenan);

  EXPECT_EQ((SampleList{
                {1, 1.1},
                {2, 1.2},
                {3, STALE_NAN},
                {4, 1.2},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

class FinalizeChunkTestFixture : public EncoderTestTrait<4>, public testing::Test {
 protected:
  static constexpr double kIntegerValue = 1.0;
  static constexpr double kDoubleValue = 1.1;
  static constexpr double kFloat32Value = -1.0;

  template <class SamplesAsserter>
  void assert_result(uint32_t ls_id, SamplesAsserter&& samples_asserter) {
    auto& open_chunk = chunk(ls_id);
    EXPECT_EQ(1U, open_chunk_timestamp(ls_id).count());
    EXPECT_EQ(BareBones::Vector<int64_t>{kSamplesPerChunk}, decode_open_chunk_timestamp_list(ls_id));

    auto finalized = finalized_chunks(ls_id);
    ASSERT_NE(finalized, nullptr);
    samples_asserter(*finalized, open_chunk);
  }
};

TEST_F(FinalizeChunkTestFixture, FinalizeUint32ConstantChunkWithUniqueTimeserie) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kIntegerValue);
  }

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kUint32Constant, open_chunk.encoding_state.encoding_type);
    ASSERT_EQ(EncodingType::kUint32Constant, finalized_chunks.front().encoding_state.encoding_type);
    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = kIntegerValue},
                      {.timestamp = 1, .value = kIntegerValue},
                      {.timestamp = 2, .value = kIntegerValue},
                      {.timestamp = 3, .value = kIntegerValue},
                  },
                  {
                      {.timestamp = 4, .value = kIntegerValue},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeUint32ConstantChunkWithNonUniqueTimeserie) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kIntegerValue);
    encoder_.encode(1, i, kIntegerValue);
  }

  // Assert
  const auto samples_asserter = [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kUint32Constant, open_chunk.encoding_state.encoding_type);
    ASSERT_EQ(EncodingType::kUint32Constant, finalized_chunks.front().encoding_state.encoding_type);
    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = kIntegerValue},
                      {.timestamp = 1, .value = kIntegerValue},
                      {.timestamp = 2, .value = kIntegerValue},
                      {.timestamp = 3, .value = kIntegerValue},
                  },
                  {
                      {.timestamp = 4, .value = kIntegerValue},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  };
  assert_result(0, samples_asserter);
  assert_result(1, samples_asserter);
}

TEST_F(FinalizeChunkTestFixture, FinalizeFloat32ConstantChunk) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kFloat32Value);
  }

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kFloat32Constant, open_chunk.encoding_state.encoding_type);
    ASSERT_EQ(EncodingType::kFloat32Constant, finalized_chunks.front().encoding_state.encoding_type);
    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = kFloat32Value},
                      {.timestamp = 1, .value = kFloat32Value},
                      {.timestamp = 2, .value = kFloat32Value},
                      {.timestamp = 3, .value = kFloat32Value},
                  },
                  {
                      {.timestamp = 4, .value = kFloat32Value},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeDoubleConstantChunk) {
  // Arrange

  // Act
  for (uint8_t i = 0; i <= kSamplesPerChunk; ++i) {
    encoder_.encode(0, i, kDoubleValue);
  }

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kDoubleConstant, open_chunk.encoding_state.encoding_type);
    ASSERT_EQ(EncodingType::kDoubleConstant, finalized_chunks.front().encoding_state.encoding_type);
    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = kDoubleValue},
                      {.timestamp = 1, .value = kDoubleValue},
                      {.timestamp = 2, .value = kDoubleValue},
                      {.timestamp = 3, .value = kDoubleValue},
                  },
                  {
                      {.timestamp = 4, .value = kDoubleValue},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeTwoDoubleConstantChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, kDoubleValue);
  for (uint8_t i = 0; i < kSamplesPerChunk; ++i) {
    encoder_.encode(0, i + 1, kDoubleValue + 0.1);
  }

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kDoubleConstant, open_chunk.encoding_state.encoding_type);
    ASSERT_EQ(EncodingType::kTwoDoubleConstant, finalized_chunks.front().encoding_state.encoding_type);

    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = kDoubleValue},
                      {.timestamp = 1, .value = kDoubleValue + 0.1},
                      {.timestamp = 2, .value = kDoubleValue + 0.1},
                      {.timestamp = 3, .value = kDoubleValue + 0.1},
                  },
                  {
                      {.timestamp = 4, .value = kDoubleValue + 0.1},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeAscIntegerValuesGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 1, 2.0);
  encoder_.encode(0, 2, 3.0);
  encoder_.encode(0, 3, 4.0);
  encoder_.encode(0, 4, 5.0);

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kUint32Constant, open_chunk.encoding_state.encoding_type);
    EXPECT_EQ(EncodingType::kAscIntegerValuesGorilla, finalized_chunks.front().encoding_state.encoding_type);

    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = 1.0},
                      {.timestamp = 1, .value = 2.0},
                      {.timestamp = 2, .value = 3.0},
                      {.timestamp = 3, .value = 4.0},
                  },
                  {
                      {.timestamp = 4, .value = 5.0},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeValuesGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.1);
  encoder_.encode(1, 0, 1.1);

  encoder_.encode(0, 1, 2.1);
  encoder_.encode(1, 1, 2.1);

  encoder_.encode(0, 2, 3.1);
  encoder_.encode(0, 3, 4.1);
  encoder_.encode(0, 4, 5.1);

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kDoubleConstant, open_chunk.encoding_state.encoding_type);
    EXPECT_EQ(EncodingType::kValuesGorilla, finalized_chunks.front().encoding_state.encoding_type);

    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = 1.1},
                      {.timestamp = 1, .value = 2.1},
                      {.timestamp = 2, .value = 3.1},
                      {.timestamp = 3, .value = 4.1},
                  },
                  {
                      {.timestamp = 4, .value = 5.1},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

TEST_F(FinalizeChunkTestFixture, FinalizeGorillaChunk) {
  // Arrange

  // Act
  encoder_.encode(0, 0, 1.1);
  encoder_.encode(0, 1, 2.1);
  encoder_.encode(0, 2, 3.1);
  encoder_.encode(0, 3, 4.1);
  encoder_.encode(0, 4, 5.1);

  // Assert
  assert_result(0, [this](const FinalizedChunkList& finalized_chunks, const DataChunk& open_chunk) {
    ASSERT_EQ(EncodingType::kDoubleConstant, open_chunk.encoding_state.encoding_type);
    EXPECT_EQ(EncodingType::kGorilla, finalized_chunks.front().encoding_state.encoding_type);

    EXPECT_EQ((ListOfSampleList{
                  {
                      {.timestamp = 0, .value = 1.1},
                      {.timestamp = 1, .value = 2.1},
                      {.timestamp = 2, .value = 3.1},
                      {.timestamp = 3, .value = 4.1},
                  },
                  {
                      {.timestamp = 4, .value = 5.1},
                  },
              }),
              Decoder::decode_chunks(storage_, finalized_chunks, open_chunk));
  });
}

class EncodeOutdatedChunkTestFixture : public EncoderTestTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

TEST_F(EncodeOutdatedChunkTestFixture, EncodeUint32ConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.0);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeUint32ConstantNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(1.0, chunk(0).encoder.uint32_constant.value());
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeUint32ConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kUint32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{2}, decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 2.0}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeFloat32ConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 1, -1.0);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeFloat32ConstantNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 1, -2.0);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(-1.0, chunk(0).encoder.float32_constant.value());
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeFloat32ConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, -1.0);
  encoder_.encode(0, 1, -1.0);
  encoder_.encode(0, 1, -2.0);

  // Assert
  ASSERT_EQ(EncodingType::kFloat32Constant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{2}, decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = -1.0}, {.timestamp = 1, .value = -2.0}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeDoubleConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeDoubleConstantNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 1, 1.2);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(1.1, storage_.double_constant_encoders[chunk(0).encoder.external_index].value());
  EXPECT_EQ(BareBones::Vector<int64_t>{1}, decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeDoubleConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 1, 1.2);

  // Assert
  ASSERT_EQ(EncodingType::kDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ(BareBones::Vector<int64_t>{2}, decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.1}, {.timestamp = 1, .value = 1.2}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeTwoDoubleConstantActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.2);
  encoder_.encode(0, 2, 1.2);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeTwoDoubleConstantNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 1.2);
  encoder_.encode(0, 2, 1.3);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  auto& encoder = storage_.two_double_constant_encoders[chunk(0).encoder.external_index];
  EXPECT_EQ(1.1, encoder.value1());
  EXPECT_EQ(1, encoder.value1_count());
  EXPECT_EQ(1.2, encoder.value2());
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeTwoDoubleConstantOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 1.2);
  encoder_.encode(0, 1, 1.2);
  encoder_.encode(0, 1, 1.3);

  // Assert
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.2}, {.timestamp = 1, .value = 1.3}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeIntegerValuesGorillaEncoderActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeIntegerValuesGorillaEncoderNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, STALE_NAN},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeIntegerValuesGorillaEncoderOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.0);
  encoder_.encode(0, 3, 2.0);
  encoder_.encode(0, 4, 3.0);
  encoder_.encode(0, 5, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 2.0);

  // Assert
  ASSERT_EQ(EncodingType::kAscIntegerValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3, 4, 5}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 2.0}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeValuesGorillaActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(1, 2, 2.1);
  encoder_.encode(0, 3, 3.1);
  encoder_.encode(1, 3, 3.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeValuesGorillaNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(1, 2, 2.1);
  encoder_.encode(0, 3, 3.1);
  encoder_.encode(1, 3, 3.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, 4.0);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_TRUE(std::ranges::equal((SampleList{
                                     {.timestamp = 1, .value = 1.1},
                                     {.timestamp = 2, .value = 2.1},
                                     {.timestamp = 3, .value = 3.1},
                                     {.timestamp = 4, .value = STALE_NAN},
                                 }),
                                 Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0))));
  EXPECT_EQ((BareBones::Vector<int64_t>{1, 2, 3, 4}), decode_open_chunk_timestamp_list(0));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeValuesGorillaOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(1, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(1, 3, 2.1);
  encoder_.encode(0, 4, 3.1);
  encoder_.encode(1, 4, 3.1);
  encoder_.encode(0, 5, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(EncodingType::kValuesGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((BareBones::Vector<int64_t>{2, 3, 4, 5}), decode_open_chunk_timestamp_list(0));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 1.1}}), Decoder::decode_outdated_chunk(*outdated));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeGorillaActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(0, 3, 3.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, STALE_NAN);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.1}, {.timestamp = 2, .value = 2.1}, {.timestamp = 3, .value = 3.1}, {.timestamp = 4, .value = STALE_NAN}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeGorillaNonActualSample) {
  // Arrange

  // Act
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(0, 3, 3.1);
  encoder_.encode(0, 4, STALE_NAN);
  encoder_.encode(0, 4, 3.0);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.1}, {.timestamp = 2, .value = 2.1}, {.timestamp = 3, .value = 3.1}, {.timestamp = 4, .value = STALE_NAN}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));
}

TEST_F(EncodeOutdatedChunkTestFixture, EncodeGorillaOutdatedSample) {
  // Arrange

  // Act
  encoder_.encode(0, 2, 1.1);
  encoder_.encode(0, 3, 2.1);
  encoder_.encode(0, 4, 3.1);
  encoder_.encode(0, 5, STALE_NAN);
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 1, 1.1);

  // Assert
  ASSERT_EQ(EncodingType::kGorilla, chunk(0).encoding_state.encoding_type);
  EXPECT_EQ((SampleList{{.timestamp = 2, .value = 1.1}, {.timestamp = 3, .value = 2.1}, {.timestamp = 4, .value = 3.1}, {.timestamp = 5, .value = STALE_NAN}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, chunk(0)));

  auto outdated = outdated_chunk(0);
  ASSERT_NE(nullptr, outdated);
  EXPECT_EQ((SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 1, .value = 1.1}}), Decoder::decode_outdated_chunk(*outdated));
}

class EraseChunkTestFixture : public EncoderTestTrait<series_data::kSamplesPerChunkDefault>, public testing::Test {};

struct DataChunkInfo {
  DataChunk chunk;
  uint32_t series_id;
  series_data::chunk::DataChunk::Type type;

  bool operator==(const DataChunkInfo&) const noexcept = default;
};

TEST_F(EraseChunkTestFixture, EraseUint32Encoder) {
  // Arrange
  encoder_.encode(0, 0, 1.0);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_NE(ts_before, ts_after);
}

TEST_F(EraseChunkTestFixture, EraseFloat32Encoder) {
  // Arrange
  encoder_.encode(0, 0, -1.0);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_NE(ts_before, ts_after);
}

TEST_F(EraseChunkTestFixture, EraseDoubleEncoder) {
  // Arrange
  encoder_.encode(0, 0, 1.1);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_NE(ts_before, ts_after);
  ASSERT_THROW({ storage_.double_constant_encoders.at(0); }, BareBones::Exception);
}

TEST_F(EraseChunkTestFixture, EraseTwoDoubleEncoder) {
  // Arrange
  encoder_.encode(0, 0, 1.1);
  encoder_.encode(0, 1, 1.2);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_NE(ts_before, ts_after);
  ASSERT_THROW({ storage_.two_double_constant_encoders.at(0); }, BareBones::Exception);
}

TEST_F(EraseChunkTestFixture, EraseAscEncoder) {
  // Arrange
  encoder_.encode(0, 0, 1.0);
  encoder_.encode(0, 1, 2.0);
  encoder_.encode(0, 2, 3.0);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_NE(ts_before, ts_after);
  ASSERT_THROW({ storage_.asc_integer_values_gorilla_encoders.at(0); }, BareBones::Exception);
}

TEST_F(EraseChunkTestFixture, EraseValuesGorillaEncoder) {
  // Arrange
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(1, 1, 1.0);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(1, 2, 1.0);
  encoder_.encode(0, 3, 3.1);
  encoder_.encode(1, 3, 1.0);

  // Act
  auto ts_before = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));
  auto ts_after = storage_.timestamp_encoder.get_state(chunk(0).timestamp_encoder_state_id).timestamp();

  // Assert
  ASSERT_EQ(ts_before, ts_after);  // timestamp series is non-unique, so not erased
  ASSERT_THROW({ storage_.values_gorilla_encoders.at(0); }, BareBones::Exception);
}

TEST_F(EraseChunkTestFixture, EraseGorillaEncoder) {
  // Arrange
  encoder_.encode(0, 1, 1.1);
  encoder_.encode(0, 2, 2.1);
  encoder_.encode(0, 3, 3.1);

  // Act
  storage_.erase_chunk<DataChunk::Type::kOpen>(chunk(0));

  // Assert
  ASSERT_THROW({ storage_.gorilla_encoders.at(0); }, BareBones::Exception);
}

}  // namespace
