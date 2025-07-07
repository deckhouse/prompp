#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

namespace {

using series_data::ChunkFinalizer;
using series_data::Decoder;
using series_data::chunk::DataChunk;
using series_data::encoder::SampleList;
using series_data::unloading::Loader;

class LoaderUnloaderTrait {
 protected:
  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  series_data::unloading::Unloader unloader_{storage_};
  BareBones::ShrinkedToFitOStringStream stream1_;
  BareBones::ShrinkedToFitOStringStream stream2_;
};

class LoaderUnloaderTestFixture : public LoaderUnloaderTrait, public testing::Test {
 protected:
  template <class... Spans>
  void load(const std::vector<uint32_t>& ls_ids, Spans&&... spans) {
    Loader loader(storage_, ls_ids);
    (..., loader.load_next(std::forward<Spans>(spans)));
    loader.load_finalize();
  }
};

TEST_F(LoaderUnloaderTestFixture, Empty) {
  // Arrange

  // Act
  unloader_.unload(stream1_);

  // Assert
  ASSERT_EQ(stream1_.view().size(), unloader_.get_empty_unloader_size_in_bytes());
}

TEST_F(LoaderUnloaderTestFixture, UnloadOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  const uint32_t chunk_stream_size_in_bits =
      storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits();

  // Act
  unloader_.unload(stream1_);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unloader_.unload(stream1_);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadTwoOpenChunks) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(100, 1, 10.0);
  encoder_.encode(100, 2, 20.0);
  encoder_.encode(100, 3, 30.0);
  encoder_.encode(100, 4, 40.0);
  encoder_.encode(100, 5, 50.0);

  unloader_.unload(stream1_);

  // Act
  load({0, 100}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_EQ((SampleList{
                {1, 10.0},
                {2, 20.0},
                {3, 30.0},
                {4, 40.0},
                {5, 50.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[100]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(100));
}

TEST_F(LoaderUnloaderTestFixture, SkipOneUnloading) {
  // Arrange
  encoder_.encode(0, 1, 1.0);

  unloader_.unload(stream1_);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);

  unloader_.unload(stream2_);

  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);
  encoder_.encode(0, 7, 7.0);

  // Act
  load({0}, stream1_.span<uint8_t>(), stream2_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}, {6, 6.0}, {7, 7.0}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unloader_.unload(stream1_);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}),
            Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.at(0).front()));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkMergeOutdated) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(0, 0, 0.0);

  unloader_.unload(stream1_);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {0, 0.0},
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
  ASSERT_TRUE(storage_.outdated_chunks.empty());
}

TEST_F(LoaderUnloaderTestFixture, LoadFinalizedChunkMergeOutdated) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(0, 0, 0.0);

  unloader_.unload(stream1_);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {0, 0.0},
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.at(0).front()));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
  ASSERT_TRUE(storage_.outdated_chunks.empty());
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkSameChunkId) {
  // Arrange

  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unloader_.unload(stream1_);

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unloader_.unload(stream2_);

  encoder_.encode(0, 7, 7.0);
  encoder_.encode(0, 8, 8.0);
  encoder_.encode(0, 9, 9.0);

  // Act
  load({0}, stream1_.span<uint8_t>(), stream2_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
                {6, 6.0},
                {7, 7.0},
                {8, 8.0},
                {9, 9.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadChunkChangeChunkId) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unloader_.unload(stream1_);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unloader_.unload(stream2_);

  encoder_.encode(0, 7, 7.0);
  encoder_.encode(0, 8, 8.0);
  encoder_.encode(0, 9, 9.0);

  // Act
  load({0}, stream1_.span<uint8_t>(), stream2_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{
                {4, 4.0},
                {5, 5.0},
                {6, 6.0},
                {7, 7.0},
                {8, 8.0},
                {9, 9.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unloader_.unload(stream1_);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscInteger, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerTheGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.1);

  unloader_.unload(stream1_);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscIntegerThenValuesGorilla, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.1}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadValuesGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(1, 3, 3.0);

  unloader_.unload(stream1_);

  // Act
  load({1}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kValuesGorilla, storage_.open_chunks[1].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.1}, {2, 2.0}, {3, 3.0}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[1]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.contains(1));
}

}  // namespace