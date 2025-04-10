#include <gmock/gmock.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "series_data/serialization/deserializer.h"
#include "series_data/serialization/serializer.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::EncodingType;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::querier::QueriedChunk;
using series_data::querier::QueriedChunkList;
using series_data::serialization::Deserializer;
using series_data::serialization::Serializer;

class SerializerDeserializerTrait {
 protected:
  DataStorage storage_;
  Serializer serializer_{storage_};
  std::chrono::system_clock clock_;
  OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder_{clock_};
  Encoder<decltype(outdated_sample_encoder_)> encoder_{storage_, outdated_sample_encoder_};
  BareBones::ShrinkedToFitOStringStream stream_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> get_buffer() const noexcept {
    return {reinterpret_cast<const uint8_t*>(stream_.view().data()), stream_.view().size()};
  }

  template <class DecodeIterator>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static SampleList decode_chunk(DecodeIterator iterator) {
    SampleList result;
    std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_insert_iterator(result));
    return result;
  }
};

class SerializerDeserializerFixture : public SerializerDeserializerTrait, public testing::Test {};

TEST_F(SerializerDeserializerFixture, EmptyChunksList) {
  // Arrange

  // Act
  serializer_.serialize({}, stream_);
  const Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(0U, deserializer.get_chunks().size());
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
  serializer_.serialize({QueriedChunk{0}, QueriedChunk{1}}, stream_);
  const Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(2U, deserializer.get_chunks().size());
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  EXPECT_EQ(deserializer.get_chunks()[0].timestamps_offset, deserializer.get_chunks()[1].timestamps_offset);
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
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
  serializer_.serialize({QueriedChunk{0}, QueriedChunk{1}, QueriedChunk{2}}, stream_);
  const Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(3U, deserializer.get_chunks().size());
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[2].encoding_state.encoding_type);
  EXPECT_EQ(deserializer.get_chunks()[0].timestamps_offset, deserializer.get_chunks()[1].timestamps_offset);
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 1.0},
          {.timestamp = 2, .value = 1.0},
          {.timestamp = 3, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 1, .value = 2.0},
          {.timestamp = 2, .value = 2.0},
          {.timestamp = 3, .value = 2.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[2]))));
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
  serializer_.serialize(stream_);
  Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(10U, deserializer.get_chunks().size());
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, deserializer.get_chunks()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, deserializer.get_chunks()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, deserializer.get_chunks()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, deserializer.get_chunks()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, deserializer.get_chunks()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, deserializer.get_chunks()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 101, .value = 1.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = 1.2},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[2]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.0},
          {.timestamp = 105, .value = 2.0},
          {.timestamp = 106, .value = 3.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[3]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
          {.timestamp = 109, .value = 3.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[4]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 110, .value = 1.1},
          {.timestamp = 111, .value = 2.1},
          {.timestamp = 112, .value = 3.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[5]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 113, .value = 2.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[6]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 114, .value = -1.0},
          {.timestamp = 115, .value = -1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[7]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 120, .value = 1.0},
          {.timestamp = 121, .value = 2.0},
          {.timestamp = 122, .value = 3.0},
          {.timestamp = 123, .value = 4.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[8]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[9]))));
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
  serializer_.serialize(stream_);
  Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(10U, deserializer.get_chunks().size());
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, deserializer.get_chunks()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, deserializer.get_chunks()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, deserializer.get_chunks()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, deserializer.get_chunks()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, deserializer.get_chunks()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, deserializer.get_chunks()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 101, .value = 1.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = 1.2},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[2]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.0},
          {.timestamp = 105, .value = 2.0},
          {.timestamp = 106, .value = 3.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[3]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
          {.timestamp = 109, .value = 3.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[4]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 110, .value = 1.1},
          {.timestamp = 111, .value = 2.1},
          {.timestamp = 112, .value = 3.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[5]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 113, .value = 2.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[6]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 114, .value = -1.0},
          {.timestamp = 115, .value = -1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[7]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 120, .value = 1.0},
          {.timestamp = 121, .value = 2.0},
          {.timestamp = 122, .value = 3.0},
          {.timestamp = 123, .value = 4.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[8]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.1},
          {.timestamp = 108, .value = 2.1},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[9]))));
}

TEST_F(SerializerDeserializerFixture, ChunkWithFinalizedTimestampStream) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(1, 100, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  // Act
  serializer_.serialize({QueriedChunk{1}}, stream_);
  const Deserializer deserializer(get_buffer());

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
}

TEST_F(SerializerDeserializerFixture, MultipleChunksOnOneSeriesId) {
  // Arrange
  encoder_.encode(0, 100, 1.0);
  encoder_.encode(0, 101, 1.0);
  encoder_.encode(0, 102, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 103, 1.0);

  // Act
  serializer_.serialize(stream_);
  const Deserializer deserializer(get_buffer());

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = 1.0},
          {.timestamp = 102, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 103, .value = 1.0},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
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
  serializer_.serialize(stream_);
  Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(10U, deserializer.get_chunks().size());
  EXPECT_TRUE(std::ranges::all_of(deserializer.get_chunks(), [](const auto& chunk) { return chunk.encoding_state.has_last_stalenan; }));
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, deserializer.get_chunks()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, deserializer.get_chunks()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, deserializer.get_chunks()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, deserializer.get_chunks()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, deserializer.get_chunks()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, deserializer.get_chunks()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.1},
          {.timestamp = 105, .value = 1.2},
          {.timestamp = 106, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[2]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.0},
          {.timestamp = 108, .value = 2.0},
          {.timestamp = 109, .value = 3.0},
          {.timestamp = 110, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[3]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = 3.1},
          {.timestamp = 114, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[4]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 115, .value = 1.1},
          {.timestamp = 116, .value = 2.1},
          {.timestamp = 117, .value = 3.1},
          {.timestamp = 118, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[5]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 119, .value = 2.0},
          {.timestamp = 120, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[6]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 121, .value = -1.0},
          {.timestamp = 122, .value = -1.0},
          {.timestamp = 123, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[7]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 130, .value = 1.0},
          {.timestamp = 131, .value = 2.0},
          {.timestamp = 132, .value = 3.0},
          {.timestamp = 133, .value = 4.1},
          {.timestamp = 134, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[8]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[9]))));
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
  serializer_.serialize(stream_);
  Deserializer deserializer(get_buffer());

  // Assert
  ASSERT_TRUE(deserializer.is_valid());
  ASSERT_EQ(10U, deserializer.get_chunks().size());
  EXPECT_TRUE(std::ranges::all_of(deserializer.get_chunks(), [](const auto& chunk) { return chunk.encoding_state.has_last_stalenan; }));
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[0].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kDoubleConstant, deserializer.get_chunks()[1].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[2].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscInteger, deserializer.get_chunks()[3].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kValuesGorilla, deserializer.get_chunks()[4].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kGorilla, deserializer.get_chunks()[5].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kUint32Constant, deserializer.get_chunks()[6].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kFloat32Constant, deserializer.get_chunks()[7].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kAscIntegerThenValuesGorilla, deserializer.get_chunks()[8].encoding_state.encoding_type);
  ASSERT_EQ(EncodingType::kTwoDoubleConstant, deserializer.get_chunks()[9].encoding_state.encoding_type);
  ASSERT_EQ(20U, deserializer.get_chunks()[9].label_set_id);

  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 100, .value = 1.0},
          {.timestamp = 101, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[0]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 102, .value = 1.1},
          {.timestamp = 103, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[1]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 104, .value = 1.1},
          {.timestamp = 105, .value = 1.2},
          {.timestamp = 106, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[2]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 107, .value = 1.0},
          {.timestamp = 108, .value = 2.0},
          {.timestamp = 109, .value = 3.0},
          {.timestamp = 110, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[3]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = 3.1},
          {.timestamp = 114, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[4]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 115, .value = 1.1},
          {.timestamp = 116, .value = 2.1},
          {.timestamp = 117, .value = 3.1},
          {.timestamp = 118, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[5]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 119, .value = 2.0},
          {.timestamp = 120, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[6]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 121, .value = -1.0},
          {.timestamp = 122, .value = -1.0},
          {.timestamp = 123, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[7]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 130, .value = 1.0},
          {.timestamp = 131, .value = 2.0},
          {.timestamp = 132, .value = 3.0},
          {.timestamp = 133, .value = 4.1},
          {.timestamp = 134, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[8]))));
  EXPECT_TRUE(std::ranges::equal(
      SampleList{
          {.timestamp = 111, .value = 1.1},
          {.timestamp = 112, .value = 2.1},
          {.timestamp = 113, .value = STALE_NAN},
      },
      decode_chunk(deserializer.create_decode_iterator(deserializer.get_chunks()[9]))));
}

class DeserializerIteratorFixture : public SerializerDeserializerTrait, public testing::Test {
 protected:
  using DecodedChunks = std::vector<SampleList>;

  DecodedChunks decode_chunks() const {
    DecodedChunks result;
    for (auto& chunk : Deserializer{get_buffer()}) {
      result.emplace_back(decode_chunk(chunk.decode_iterator()));
    }
    return result;
  }
};

TEST_F(DeserializerIteratorFixture, EmptyChunksList) {
  // Arrange

  // Act
  serializer_.serialize({}, stream_);
  auto decoded_chunks = decode_chunks();

  // Assert
  EXPECT_TRUE(std::ranges::equal(DecodedChunks{}, decoded_chunks));
}

TEST_F(DeserializerIteratorFixture, OneChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 1.0);

  // Act
  serializer_.serialize({QueriedChunk{0}}, stream_);
  auto decoded_chunks = decode_chunks();

  // Assert
  EXPECT_TRUE(std::ranges::equal(DecodedChunks{SampleList{{.timestamp = 1, .value = 1.0}, {.timestamp = 2, .value = 1.0}}}, decoded_chunks));
}

TEST_F(DeserializerIteratorFixture, TwoChunks) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 2, 1.0);

  // Act
  serializer_.serialize({QueriedChunk{0}, QueriedChunk{1}}, stream_);
  auto decoded_chunks = decode_chunks();

  // Assert
  EXPECT_TRUE(std::ranges::equal(DecodedChunks{SampleList{{.timestamp = 1, .value = 1.0}}, SampleList{{.timestamp = 2, .value = 1.0}}}, decoded_chunks));
}

}  // namespace