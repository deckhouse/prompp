#include <gmock/gmock.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/serialization/deserializer.h"
#include "series_data/serialization/serialized_data.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::ChunkFinalizer;
using series_data::DataStorage;
using Encoder = series_data::Encoder<>;
using series_data::EncodingType;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::querier::QueriedChunk;
using series_data::querier::QueriedChunkList;
using series_data::serialization::DataSerializer;
using series_data::serialization::SerializedData;
using series_data::serialization::SerializedDataView;

class SerializerDeserializerTrait {
 protected:
  DataStorage storage_;
  Encoder encoder_{storage_};
  DataSerializer serializer_{storage_};

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SampleList decode_current_chunk(SerializedDataView& data, uint32_t series_id) {
    EXPECT_EQ(series_id, data.next_series().first);

    SampleList result;
    std::ranges::copy(data.create_current_series_iterator(), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static SampleList decode_chunk_by_id(const SerializedDataView& data, uint32_t series_chunk_id) {
    SampleList result;
    std::ranges::copy(data.create_series_iterator(series_chunk_id), DecodeIteratorSentinel{}, std::back_insert_iterator(result));
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedData serialize(const QueriedChunkList& queried_chunks) noexcept {
    auto data = serializer_.serialize(queried_chunks);
    storage_.reset();
    return data;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedData serialize() noexcept {
    auto data = serializer_.serialize();
    storage_.reset();
    return data;
  }
};

class SerializerDeserializerFixture : public SerializerDeserializerTrait, public testing::Test {};

TEST_F(SerializerDeserializerFixture, EmptyChunksList) {
  // Arrange

  // Act
  const auto serialized = serialize({});
  const SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(0U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(DataStorage::CompactBitSequence::reserved_bytes_for_reader().size(), serialized_view.get_buffer_view().size());
}

TEST_F(SerializerDeserializerFixture, TwoUint32ConstantChunkWithCommonTimestampStream) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.0);

  // Act
  const auto serialized = serialize({QueriedChunk{0}, QueriedChunk{1}});
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(2U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  EXPECT_EQ(serialized_view.get_chunks_view()[0].timestamps_offset, serialized_view.get_chunks_view()[1].timestamps_offset);
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 1)));
}

TEST_F(SerializerDeserializerFixture, TwoUint32ConstantFinalizedChunkWithCommonTimestampStream) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.0);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);
  encoder_.encode(0, 4, 1.0);
  encoder_.encode(1, 4, 1.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(4U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[3].encoding_state.encoding_type);
  EXPECT_EQ(serialized_view.get_chunks_view()[0].timestamps_offset, serialized_view.get_chunks_view()[2].timestamps_offset);
  EXPECT_EQ(serialized_view.get_chunks_view()[1].timestamps_offset, serialized_view.get_chunks_view()[3].timestamps_offset);
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
          {.timestamp = 4, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
          {.timestamp = 4, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 1)));
}

TEST_F(SerializerDeserializerFixture, ThreeUint32ConstantChunkWithCommonAndUniqueTimestampStream) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.0);

  encoder_.encode(2, 1, 2.0);
  encoder_.encode(2, 2, 2.0);
  encoder_.encode(2, 3, 2.0);

  // Act
  const auto serialized = serialize({QueriedChunk{0}, QueriedChunk{1}, QueriedChunk{2}});
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(3U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  EXPECT_EQ(serialized_view.get_chunks_view()[0].timestamps_offset, serialized_view.get_chunks_view()[1].timestamps_offset);
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 1)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 2.0},
          {.timestamp = 2, .value = 2.0},
          {.timestamp = 3, .value = 2.0},
      },
      decode_current_chunk(serialized_view, 2)));
}

TEST_F(SerializerDeserializerFixture, AllChunkTypes) {
  // Arrange
  encoder_.encode(0, 100, 1.0);

  encoder_.encode(1, 101, 1.1);

  encoder_.encode(2, 102, 1.1);
  encoder_.encode(2, 103, 1.2);

  encoder_.encode(3, 104, 1.0);
  encoder_.encode(3, 105, 2.0);
  encoder_.encode(3, 106, 3.0);

  encoder_.encode(4, 107, 1.1);
  encoder_.encode(20, 107, 1.1);
  encoder_.encode(4, 108, 2.1);
  encoder_.encode(20, 108, 2.1);
  encoder_.encode(4, 109, 3.1);

  encoder_.encode(5, 110, 1.1);
  encoder_.encode(5, 111, 2.1);
  encoder_.encode(5, 112, 3.1);

  encoder_.encode(6, 113, 2.0);

  encoder_.encode(7, 114, -1.0);
  encoder_.encode(7, 115, -1.0);

  encoder_.encode(8, 120, 1.0);
  encoder_.encode(8, 121, 2.0);
  encoder_.encode(8, 122, 3.0);
  encoder_.encode(8, 123, 4.1);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(10U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, serialized_view.get_chunks_view()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, serialized_view.get_chunks_view()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, serialized_view.get_chunks_view()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, serialized_view.get_chunks_view()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, serialized_view.get_chunks_view()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, serialized_view.get_chunks_view()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 101, .value = 1.1},
      },
      decode_current_chunk(serialized_view, 1)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = 1.2},
      },
      decode_current_chunk(serialized_view, 2)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.0},
          {.timestamp = 105, .value = 2.0},
          {.timestamp = 106, .value = 3.0},
      },
      decode_current_chunk(serialized_view, 3)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
          {.timestamp = 109, .value = 3.1},
      },
      decode_current_chunk(serialized_view, 4)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 110, .value = 1.1},
          {.timestamp = 111, .value = 2.1},
          {.timestamp = 112, .value = 3.1},
      },
      decode_current_chunk(serialized_view, 5)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 113, .value = 2.0},
      },
      decode_current_chunk(serialized_view, 6)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 114, .value = -1.0},
          {.timestamp = 115, .value = -1.0},
      },
      decode_current_chunk(serialized_view, 7)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 120, .value = 1.0},
          {.timestamp = 121, .value = 2.0},
          {.timestamp = 122, .value = 3.0},
          {.timestamp = 123, .value = 4.1},
      },
      decode_current_chunk(serialized_view, 8)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
      },
      decode_current_chunk(serialized_view, 20)));
}

TEST_F(SerializerDeserializerFixture, FinalizedAllChunkTypes) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(1, 101, 1.1);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);

  encoder_.encode(2, 102, 1.1);
  encoder_.encode(2, 103, 1.2);
  ChunkFinalizer::finalize(storage_, 2, storage_.open_chunks[2]);

  encoder_.encode(3, 104, 1.0);
  encoder_.encode(3, 105, 2.0);
  encoder_.encode(3, 106, 3.0);
  ChunkFinalizer::finalize(storage_, 3, storage_.open_chunks[3]);

  encoder_.encode(4, 107, 1.1);
  encoder_.encode(20, 107, 1.1);
  encoder_.encode(4, 108, 2.1);
  encoder_.encode(20, 108, 2.1);
  encoder_.encode(4, 109, 3.1);
  ChunkFinalizer::finalize(storage_, 4, storage_.open_chunks[4]);
  ChunkFinalizer::finalize(storage_, 20, storage_.open_chunks[20]);

  encoder_.encode(5, 110, 1.1);
  encoder_.encode(5, 111, 2.1);
  encoder_.encode(5, 112, 3.1);
  ChunkFinalizer::finalize(storage_, 5, storage_.open_chunks[5]);

  encoder_.encode(6, 113, 2.0);
  ChunkFinalizer::finalize(storage_, 6, storage_.open_chunks[6]);

  encoder_.encode(7, 114, -1.0);
  encoder_.encode(7, 115, -1.0);
  ChunkFinalizer::finalize(storage_, 7, storage_.open_chunks[7]);

  encoder_.encode(8, 120, 1.0);
  encoder_.encode(8, 121, 2.0);
  encoder_.encode(8, 122, 3.0);
  encoder_.encode(8, 123, 4.1);
  ChunkFinalizer::finalize(storage_, 8, storage_.open_chunks[8]);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(10U, serialized_view.get_chunks_view().size());
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, serialized_view.get_chunks_view()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, serialized_view.get_chunks_view()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, serialized_view.get_chunks_view()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, serialized_view.get_chunks_view()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, serialized_view.get_chunks_view()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, serialized_view.get_chunks_view()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 101, .value = 1.1},
      },
      decode_current_chunk(serialized_view, 1)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = 1.2},
      },
      decode_current_chunk(serialized_view, 2)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.0},
          {.timestamp = 105, .value = 2.0},
          {.timestamp = 106, .value = 3.0},
      },
      decode_current_chunk(serialized_view, 3)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
          {.timestamp = 109, .value = 3.1},
      },
      decode_current_chunk(serialized_view, 4)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 110, .value = 1.1},
          {.timestamp = 111, .value = 2.1},
          {.timestamp = 112, .value = 3.1},
      },
      decode_current_chunk(serialized_view, 5)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 113, .value = 2.0},
      },
      decode_current_chunk(serialized_view, 6)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 114, .value = -1.0},
          {.timestamp = 115, .value = -1.0},
      },
      decode_current_chunk(serialized_view, 7)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 120, .value = 1.0},
          {.timestamp = 121, .value = 2.0},
          {.timestamp = 122, .value = 3.0},
          {.timestamp = 123, .value = 4.1},
      },
      decode_current_chunk(serialized_view, 8)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
      },
      decode_current_chunk(serialized_view, 20)));
}

TEST_F(SerializerDeserializerFixture, ChunkWithFinalizedTimestampStream) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(1, 100, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  // Act
  const auto serialized = serialize({QueriedChunk{1}});
  SerializedDataView serialized_view(serialized);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 1)));
}

TEST_F(SerializerDeserializerFixture, MultipleChunksOnOneSeriesId) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 102, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 1.0);
  encoder_.encode(0, 104, 1.0);
  encoder_.encode(0, 105, 1.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = 1.0},
          {.timestamp = 102, .value = 1.0},
          {.timestamp = 103, .value = 1.0},
          {.timestamp = 104, .value = 1.0},
          {.timestamp = 105, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));
}

TEST_F(SerializerDeserializerFixture, QueryFinalizedOnly) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 102, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 1.0);
  encoder_.encode(0, 104, 1.0);
  encoder_.encode(0, 105, 1.0);

  // Act
  const auto serialized = serialize({QueriedChunk{0, 0}});
  SerializedDataView serialized_view(serialized);

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = 1.0},
          {.timestamp = 102, .value = 1.0},
      },
      decode_current_chunk(serialized_view, 0)));
}

TEST_F(SerializerDeserializerFixture, MultipleChunksOnOneSeriesIdWithSeveralFinalized) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 2.0);
  encoder_.encode(0, 102, 3.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 4.0);
  encoder_.encode(0, 104, 5.0);
  encoder_.encode(0, 105, 6.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 106, 7.0);
  encoder_.encode(0, 107, 8.0);
  encoder_.encode(0, 108, 9.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  EXPECT_TRUE(std::ranges::equal(SampleList{{.timestamp = 100, .value = 1.0},
                                            {.timestamp = 101, .value = 2.0},
                                            {.timestamp = 102, .value = 3.0},
                                            {.timestamp = 103, .value = 4.0},
                                            {.timestamp = 104, .value = 5.0},
                                            {.timestamp = 105, .value = 6.0},
                                            {.timestamp = 106, .value = 7.0},
                                            {.timestamp = 107, .value = 8.0},
                                            {.timestamp = 108, .value = 9.0}},
                                 decode_current_chunk(serialized_view, 0)));
}

TEST_F(SerializerDeserializerFixture, CreateIteratorFromChunkId) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 2.0);
  encoder_.encode(0, 102, 3.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 4.0);
  encoder_.encode(0, 104, 5.0);
  encoder_.encode(0, 105, 6.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 106, 7.0);
  encoder_.encode(0, 107, 8.0);
  encoder_.encode(0, 108, 9.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  EXPECT_TRUE(std::ranges::equal(SampleList{{.timestamp = 100, .value = 1.0},
                                            {.timestamp = 101, .value = 2.0},
                                            {.timestamp = 102, .value = 3.0},
                                            {.timestamp = 103, .value = 4.0},
                                            {.timestamp = 104, .value = 5.0},
                                            {.timestamp = 105, .value = 6.0},
                                            {.timestamp = 106, .value = 7.0},
                                            {.timestamp = 107, .value = 8.0},
                                            {.timestamp = 108, .value = 9.0}},
                                 decode_chunk_by_id(serialized_view, serialized_view.next_series().second)));
}

TEST_F(SerializerDeserializerFixture, AllChunkTypesWithStalenan) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, STALE_NAN);

  encoder_.encode(1, 102, 1.1);
  encoder_.encode(1, 103, STALE_NAN);

  encoder_.encode(2, 104, 1.1);
  encoder_.encode(2, 105, 1.2);
  encoder_.encode(2, 106, STALE_NAN);

  encoder_.encode(3, 107, 1.0);
  encoder_.encode(3, 108, 2.0);
  encoder_.encode(3, 109, 3.0);
  encoder_.encode(3, 110, STALE_NAN);

  encoder_.encode(4, 111, 1.1);
  encoder_.encode(20, 111, 1.1);
  encoder_.encode(4, 112, 2.1);
  encoder_.encode(20, 112, 2.1);
  encoder_.encode(4, 113, 3.1);
  encoder_.encode(4, 114, STALE_NAN);
  encoder_.encode(20, 113, STALE_NAN);

  encoder_.encode(5, 115, 1.1);
  encoder_.encode(5, 116, 2.1);
  encoder_.encode(5, 117, 3.1);
  encoder_.encode(5, 118, STALE_NAN);

  encoder_.encode(6, 119, 2.0);
  encoder_.encode(6, 120, STALE_NAN);

  encoder_.encode(7, 121, -1.0);
  encoder_.encode(7, 122, -1.0);
  encoder_.encode(7, 123, STALE_NAN);

  encoder_.encode(8, 130, 1.0);
  encoder_.encode(8, 131, 2.0);
  encoder_.encode(8, 132, 3.0);
  encoder_.encode(8, 133, 4.1);
  encoder_.encode(8, 134, STALE_NAN);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(10U, serialized_view.get_chunks_view().size());
  EXPECT_TRUE(std::ranges::all_of(serialized_view.get_chunks_view(), [](const auto& chunk) { return chunk.encoding_state.has_last_stalenan; }));
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, serialized_view.get_chunks_view()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, serialized_view.get_chunks_view()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, serialized_view.get_chunks_view()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, serialized_view.get_chunks_view()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, serialized_view.get_chunks_view()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, serialized_view.get_chunks_view()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 0)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 1)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.1},
          {.timestamp = 105, .value = 1.2},
          {.timestamp = 106, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 2)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.0},
          {.timestamp = 108, .value = 2.0},
          {.timestamp = 109, .value = 3.0},
          {.timestamp = 110, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 3)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = 3.1},
          {.timestamp = 114, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 4)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 115, .value = 1.1},
          {.timestamp = 116, .value = 2.1},
          {.timestamp = 117, .value = 3.1},
          {.timestamp = 118, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 5)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 119, .value = 2.0},
          {.timestamp = 120, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 6)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 121, .value = -1.0},
          {.timestamp = 122, .value = -1.0},
          {.timestamp = 123, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 7)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 130, .value = 1.0},
          {.timestamp = 131, .value = 2.0},
          {.timestamp = 132, .value = 3.0},
          {.timestamp = 133, .value = 4.1},
          {.timestamp = 134, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 8)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 20)));
}

TEST_F(SerializerDeserializerFixture, FinalizedAllChunkTypesWithStalenan) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(1, 102, 1.1);
  encoder_.encode(1, 103, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);

  encoder_.encode(2, 104, 1.1);
  encoder_.encode(2, 105, 1.2);
  encoder_.encode(2, 106, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 2, storage_.open_chunks[2]);

  encoder_.encode(3, 107, 1.0);
  encoder_.encode(3, 108, 2.0);
  encoder_.encode(3, 109, 3.0);
  encoder_.encode(3, 110, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 3, storage_.open_chunks[3]);

  encoder_.encode(4, 111, 1.1);
  encoder_.encode(20, 111, 1.1);
  encoder_.encode(4, 112, 2.1);
  encoder_.encode(20, 112, 2.1);
  encoder_.encode(4, 113, 3.1);
  encoder_.encode(4, 114, STALE_NAN);
  encoder_.encode(20, 113, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 4, storage_.open_chunks[4]);
  ChunkFinalizer::finalize(storage_, 20, storage_.open_chunks[20]);

  encoder_.encode(5, 115, 1.1);
  encoder_.encode(5, 116, 2.1);
  encoder_.encode(5, 117, 3.1);
  encoder_.encode(5, 118, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 5, storage_.open_chunks[5]);

  encoder_.encode(6, 119, 2.0);
  encoder_.encode(6, 120, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 6, storage_.open_chunks[6]);

  encoder_.encode(7, 121, -1.0);
  encoder_.encode(7, 122, -1.0);
  encoder_.encode(7, 123, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 7, storage_.open_chunks[7]);

  encoder_.encode(8, 130, 1.0);
  encoder_.encode(8, 131, 2.0);
  encoder_.encode(8, 132, 3.0);
  encoder_.encode(8, 133, 4.1);
  encoder_.encode(8, 134, STALE_NAN);
  ChunkFinalizer::finalize(storage_, 8, storage_.open_chunks[8]);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  // Assert
  ASSERT_EQ(10U, serialized_view.get_chunks_view().size());
  EXPECT_TRUE(std::ranges::all_of(serialized_view.get_chunks_view(), [](const auto& chunk) { return chunk.encoding_state.has_last_stalenan; }));
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, serialized_view.get_chunks_view()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, serialized_view.get_chunks_view()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, serialized_view.get_chunks_view()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, serialized_view.get_chunks_view()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, serialized_view.get_chunks_view()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, serialized_view.get_chunks_view()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, serialized_view.get_chunks_view()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, serialized_view.get_chunks_view()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, serialized_view.get_chunks_view()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 0)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 1)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.1},
          {.timestamp = 105, .value = 1.2},
          {.timestamp = 106, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 2)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.0},
          {.timestamp = 108, .value = 2.0},
          {.timestamp = 109, .value = 3.0},
          {.timestamp = 110, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 3)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = 3.1},
          {.timestamp = 114, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 4)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 115, .value = 1.1},
          {.timestamp = 116, .value = 2.1},
          {.timestamp = 117, .value = 3.1},
          {.timestamp = 118, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 5)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 119, .value = 2.0},
          {.timestamp = 120, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 6)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 121, .value = -1.0},
          {.timestamp = 122, .value = -1.0},
          {.timestamp = 123, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 7)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 130, .value = 1.0},
          {.timestamp = 131, .value = 2.0},
          {.timestamp = 132, .value = 3.0},
          {.timestamp = 133, .value = 4.1},
          {.timestamp = 134, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 8)));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = STALE_NAN},
      },
      decode_current_chunk(serialized_view, 20)));
}

class SerializedDataNextIterFixture : public SerializerDeserializerTrait, public testing::Test {
 protected:
  static std::vector<uint32_t> get_chunks_ids(SerializedDataView& view) {
    std::vector<uint32_t> ans{};
    uint32_t id = view.next_series().first;
    while (id != SerializedDataView::kNoMoreSeries) {
      ans.push_back(id);
      id = view.next_series().first;
    }
    return ans;
  }
};

TEST_F(SerializedDataNextIterFixture, EmptyChunksList) {
  // Arrange

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  const auto ids = get_chunks_ids(serialized_view);

  // Assert
  EXPECT_TRUE(ids.empty());
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
}

TEST_F(SerializedDataNextIterFixture, OneChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  auto ids = get_chunks_ids(serialized_view);

  // Assert
  EXPECT_TRUE(std::ranges::equal(ids, std::initializer_list{0u}));
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
}

TEST_F(SerializedDataNextIterFixture, OneChunkFinalized) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, 1.0);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  auto ids = get_chunks_ids(serialized_view);

  // Assert
  EXPECT_TRUE(std::ranges::equal(ids, std::initializer_list{0u}));
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
}

TEST_F(SerializedDataNextIterFixture, SeveralChunks) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.0);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.0);

  encoder_.encode(2, 1, 2.0);
  encoder_.encode(2, 2, 2.0);
  encoder_.encode(2, 3, 2.0);

  encoder_.encode(100, 4, 2.1);
  encoder_.encode(100, 5, 2.2);
  encoder_.encode(100, 7, 2.3);

  // Act
  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  auto ids = get_chunks_ids(serialized_view);

  // Assert
  EXPECT_TRUE(std::ranges::equal(ids, std::initializer_list{0u, 1u, 2u, 100u}));
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
}

class SerializedDataIterFixture : public SerializerDeserializerTrait, public testing::Test {};

TEST_F(SerializedDataIterFixture, ResetIteratorToSameSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 3, 1.0);
  encoder_.encode(0, 4, 1.0);

  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  auto [series_id, chunk_id] = serialized_view.next_series();

  // Act
  auto iter = serialized_view.create_series_iterator(chunk_id);
  iter.reset(serialized_view.get_buffer_view(), serialized_view.get_chunks_view(), chunk_id);

  // Assert
  EXPECT_EQ(series_id, 0u);
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
  EXPECT_TRUE(std::ranges::equal(std::ranges::subrange(iter, DecodeIteratorSentinel{}),
                                 std::initializer_list{Sample{.timestamp = 1, .value = 1.0}, Sample{.timestamp = 2, .value = 1.0},
                                                       Sample{.timestamp = 3, .value = 1.0}, Sample{.timestamp = 4, .value = 1.0}}));
}

TEST_F(SerializedDataIterFixture, ResetIteratorToAnotherSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.1);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.2);

  encoder_.encode(0, 4, 1.0);
  encoder_.encode(1, 4, 1.3);

  const auto serialized = serialize();
  SerializedDataView serialized_view(serialized);

  auto [series_id0, chunk_id0] = serialized_view.next_series();
  auto [series_id1, chunk_id1] = serialized_view.next_series();

  // Act
  auto iter = serialized_view.create_series_iterator(chunk_id0);
  iter.reset(serialized_view.get_buffer_view(), serialized_view.get_chunks_view(), chunk_id1);

  // Assert
  EXPECT_EQ(series_id0, 0u);
  EXPECT_EQ(series_id1, 1u);
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, serialized_view.next_series().first);
  EXPECT_TRUE(std::ranges::equal(std::ranges::subrange(iter, DecodeIteratorSentinel{}),
                                 std::initializer_list{Sample{.timestamp = 1, .value = 1.0}, Sample{.timestamp = 2, .value = 1.1},
                                                       Sample{.timestamp = 3, .value = 1.2}, Sample{.timestamp = 4, .value = 1.3}}));
}

TEST_F(SerializedDataIterFixture, ResetIteratorToAnotherSerializedData) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.0);

  encoder_.encode(0, 2, 1.0);
  encoder_.encode(1, 2, 1.1);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  ChunkFinalizer::finalize(storage_, 1, storage_.open_chunks[1]);

  encoder_.encode(0, 3, 1.0);
  encoder_.encode(1, 3, 1.2);

  encoder_.encode(0, 4, 1.0);
  encoder_.encode(1, 4, 1.3);

  const auto serialized0 = serializer_.serialize();
  SerializedDataView serialized_view0(serialized0);

  encoder_.encode(0, 5, 1.0);
  encoder_.encode(1, 5, 1.4);

  const auto serialized1 = serialize();
  SerializedDataView serialized_view1(serialized1);

  auto [series_id0, chunk_id0] = serialized_view0.next_series();

  std::ignore = serialized_view1.next_series();
  auto [series_id1, chunk_id1] = serialized_view1.next_series();

  // Act
  auto iter = serialized_view0.create_series_iterator(chunk_id0);
  iter.reset(serialized_view1.get_buffer_view(), serialized_view1.get_chunks_view(), chunk_id1);

  // Assert
  EXPECT_TRUE(
      std::ranges::equal(std::ranges::subrange(iter, DecodeIteratorSentinel{}),
                         std::initializer_list{Sample{.timestamp = 1, .value = 1.0}, Sample{.timestamp = 2, .value = 1.1}, Sample{.timestamp = 3, .value = 1.2},
                                               Sample{.timestamp = 4, .value = 1.3}, Sample{.timestamp = 5, .value = 1.4}}));
}

}  // namespace