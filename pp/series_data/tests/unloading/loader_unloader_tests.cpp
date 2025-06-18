#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

namespace {

class LoaderUnloaderTrait {
 protected:
  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
  series_data::unloading::Unloader unloader_{storage_};

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> get_buffer() const noexcept {
    return {reinterpret_cast<const uint8_t*>(stream_.view().data()), stream_.view().size()};
  }

  void mark_series_as_unused(uint32_t ls_id) { storage_.unused_series_bitmap.add(ls_id); }
};

class LoaderUnloaderTestFixture : public LoaderUnloaderTrait, public testing::Test {
 protected:
  void SetUp() override {
    storage_.reset();
    stream_.clear();
  }
};

TEST_F(LoaderUnloaderTestFixture, Empty) {
  // Arrange

  // Act
  unloader_.unload(stream_);

  // Assert
  ASSERT_EQ(stream_.view().size(), unloader_.get_empty_unloader_size_in_bytes());
}

TEST_F(LoaderUnloaderTestFixture, UnloadOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  mark_series_as_unused(0);

  const uint32_t chunk_stream_size_in_bits =
      storage_.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits();

  // Act
  unloader_.unload(stream_);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
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

  mark_series_as_unused(0);
  mark_series_as_unused(100);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {0, 100};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_EQ((series_data::encoder::SampleList{
                {1, 10.0},
                {2, 20.0},
                {3, 30.0},
                {4, 40.0},
                {5, 50.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[100]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(100));
}

TEST_F(LoaderUnloaderTestFixture, SkipOneUnloading) {
  // Arrange
  mark_series_as_unused(0);

  encoder_.encode(0, 1, 1.0);

  unloader_.unload(stream_);

  const size_t size1 = stream_.view().size();

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);

  unloader_.unload(stream_);

  const size_t size2 = stream_.view().size();

  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);
  encoder_.encode(0, 7, 7.0);

  // Act
  std::vector<uint32_t> chunk_ids = {0};

  auto span1 = stream_.span<uint8_t>().subspan(0, size1);
  auto span2 = stream_.span<uint8_t>().subspan(size1, size2);

  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(span1);
  loader.load_next(span2);
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}, {6, 6.0}, {7, 7.0}}),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);
  // Act

  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.at(0).front()));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkMergeOutdated) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(0, 0, 0.0);

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
                {0, 0.0},
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
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

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
                {0, 0.0},
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.at(0).front()));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
  ASSERT_TRUE(storage_.outdated_chunks.empty());
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkSameChunkId) {
  // Arrange
  mark_series_as_unused(0);

  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unloader_.unload(stream_);

  const size_t size1 = stream_.view().size();

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unloader_.unload(stream_);

  const size_t size2 = stream_.view().size();

  encoder_.encode(0, 7, 7.0);
  encoder_.encode(0, 8, 8.0);
  encoder_.encode(0, 9, 9.0);

  // Act
  std::vector<uint32_t> chunk_ids = {0};

  auto span1 = stream_.span<uint8_t>().subspan(0, size1);
  auto span2 = stream_.span<uint8_t>().subspan(size1, size2);

  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(span1);
  loader.load_next(span2);
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
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
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadChunkChangeChunkId) {
  // Arrange
  mark_series_as_unused(0);

  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unloader_.unload(stream_);

  const size_t size1 = stream_.view().size();

  series_data::ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unloader_.unload(stream_);

  const size_t size2 = stream_.view().size();

  encoder_.encode(0, 7, 7.0);
  encoder_.encode(0, 8, 8.0);
  encoder_.encode(0, 9, 9.0);

  // Act
  std::vector<uint32_t> chunk_ids = {0};

  auto span1 = stream_.span<uint8_t>().subspan(0, size1);
  auto span2 = stream_.span<uint8_t>().subspan(size1, size2);

  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(span1);
  loader.load_next(span2);
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ((series_data::encoder::SampleList{
                {4, 4.0},
                {5, 5.0},
                {6, 6.0},
                {7, 7.0},
                {8, 8.0},
                {9, 9.0},
            }),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscInteger, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((series_data::encoder::SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerTheGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.1);

  mark_series_as_unused(0);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {0};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscIntegerThenValuesGorilla, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((series_data::encoder::SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.1}}),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadValuesGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(1, 3, 3.0);

  mark_series_as_unused(1);

  unloader_.unload(stream_);

  // Act
  std::vector<uint32_t> chunk_ids = {1};
  series_data::unloading::Loader loader(storage_, chunk_ids);
  loader.load_next(stream_.span<uint8_t>());
  loader.load_finalize<series_data::Encoder<>>();

  // Assert
  ASSERT_EQ(series_data::EncodingType::kValuesGorilla, storage_.open_chunks[1].encoding_state.encoding_type);
  ASSERT_EQ((series_data::encoder::SampleList{{1, 1.1}, {2, 2.0}, {3, 3.0}}),
            series_data::Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[1]));
  ASSERT_FALSE(storage_.unused_series_bitmap.contains(1));
}

}  // namespace