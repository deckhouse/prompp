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
using series_data::unloading::SeriesToLoadInfo;
using series_data::unloading::Unloader;
using std::operator""s;

class LoaderUnloaderTrait {
 protected:
  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  Unloader unloader_{storage_};
  BareBones::ShrinkedToFitOStringStream stream1_;
  BareBones::ShrinkedToFitOStringStream stream2_;

  template <class... Spans>
  void load(const std::vector<uint32_t>& ls_ids, Spans&&... spans) {
    Loader loader(storage_, ls_ids, ls_ids.size());
    (..., loader.load_next(std::forward<Spans>(spans)));
    loader.load_finalize();
  }

  void unload(BareBones::ShrinkedToFitOStringStream& stream) {
    unloader_.create_snapshot(stream);
    unloader_.unload();
  }
};

class LoaderUnloaderTestFixture : public LoaderUnloaderTrait, public testing::Test {};

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
  unload(stream1_);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);

  ASSERT_EQ(storage_.unloaded_series_bitmap.popcount(), 1);
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  const uint32_t chunk_stream_size_in_bits_before =
      storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits();

  unload(stream1_);

  // Act
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
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));

  ASSERT_EQ(chunk_stream_size_in_bits_before,
            storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits());
}

TEST_F(LoaderUnloaderTestFixture, EncodeAfterLoad) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unload(stream1_);
  load({0}, stream1_.span<uint8_t>());

  // Act
  encoder_.encode(0, 6, 10.0);
  encoder_.encode(0, 7, 20.0);
  encoder_.encode(0, 8, 30.0);

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
                {6, 10.0},
                {7, 20.0},
                {8, 30.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
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

  unload(stream1_);

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

  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(100));
}

TEST_F(LoaderUnloaderTestFixture, SkipOneUnloading) {
  // Arrange
  encoder_.encode(0, 1, 1.0);

  unload(stream1_);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);

  unload(stream2_);

  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);
  encoder_.encode(0, 7, 7.0);

  // Act
  load({0}, stream1_.span<uint8_t>(), stream2_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}, {6, 6.0}, {7, 7.0}}),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));

  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unload(stream1_);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}),
            Decoder::decode_chunk<DataChunk::Type::kFinalized>(storage_, storage_.finalized_chunks.at(0).front()));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkMergeOutdated) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(0, 0, 0.0);

  unload(stream1_);

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
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
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

  unload(stream1_);

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
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
  ASSERT_TRUE(storage_.outdated_chunks.empty());
}

TEST_F(LoaderUnloaderTestFixture, LoadOpenChunkSameChunkId) {
  // Arrange

  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload(stream1_);

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unload(stream2_);

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
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadChunkChangeChunkId) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload(stream1_);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  encoder_.encode(0, 6, 6.0);

  unload(stream2_);

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
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload(stream1_);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscInteger, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadAscIntegerTheGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.1);

  unload(stream1_);

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscIntegerThenValuesGorilla, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.1}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(LoaderUnloaderTestFixture, LoadValuesGorillaChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, 1.1);

  encoder_.encode(0, 2, 2.0);
  encoder_.encode(1, 2, 2.0);

  encoder_.encode(1, 3, 3.0);

  unload(stream1_);

  // Act
  load({1}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kValuesGorilla, storage_.open_chunks[1].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.1}, {2, 2.0}, {3, 3.0}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[1]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(1));
}

TEST_F(LoaderUnloaderTestFixture, LoadOnlyUnloadedSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload(stream1_);
  load({0}, stream1_.span<uint8_t>());

  // Act
  load({0}, stream1_.span<uint8_t>());

  // Assert
  ASSERT_EQ(series_data::EncodingType::kAscInteger, storage_.open_chunks[0].encoding_state.encoding_type);
  ASSERT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  ASSERT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

class LoaderUnloaderBigTestFixture : public LoaderUnloaderTestFixture {
 protected:
  void SetUp() override {
    encoder_.encode(0, 1, 0.0);
    encoder_.encode(0, 2, 0.0);
    encoder_.encode(0, 3, 0.0);
    encoder_.encode(0, 4, 0.0);
    encoder_.encode(0, 5, 0.0);

    encoder_.encode(1, 1, 1.0);
    encoder_.encode(1, 2, 1.0);
    encoder_.encode(1, 3, 1.0);
    encoder_.encode(1, 4, 1.0);
    encoder_.encode(1, 5, 1.0);

    encoder_.encode(2, 1, 1.0);
    encoder_.encode(2, 2, 2.0);
    encoder_.encode(2, 3, 3.0);
    encoder_.encode(2, 4, 4.0);
    encoder_.encode(2, 5, 5.0);

    encoder_.encode(3, 1, 6.0);
    encoder_.encode(3, 2, 7.0);
    encoder_.encode(3, 3, 8.0);
    encoder_.encode(3, 4, 9.0);
    encoder_.encode(3, 5, 10.0);
  }
};

TEST_F(LoaderUnloaderBigTestFixture, UnloadAfterQuery) {
  // Arrange
  storage_.queried_series_bitmap.set({0, 1, 3});
  const uint32_t chunk_stream_size_in_bits =
      storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[2].encoder.external_index).size_in_bits();

  // Act
  unload(stream1_);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[2].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);

  ASSERT_EQ(storage_.unloaded_series_bitmap.popcount(), 1);
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(2));
}

TEST_F(LoaderUnloaderBigTestFixture, EmptyLoader) {
  // Arrange
  storage_.queried_series_bitmap.set({2, 3});
  Loader loader(storage_);

  // Act
  loader.add_series_to_load(storage_.unloaded_series_bitmap, storage_.unloaded_series_bitmap.popcount());

  // Assert
  ASSERT_TRUE(loader.empty());
}

TEST_F(LoaderUnloaderBigTestFixture, LoadAll) {
  // Arrange
  storage_.queried_series_bitmap.set({0, 1});
  Loader loader(storage_);

  unload(stream1_);

  // Act
  loader.add_series_to_load(storage_.unloaded_series_bitmap, storage_.unloaded_series_bitmap.popcount());
  loader.load_next(stream1_.span<uint8_t>());
  loader.load_finalize();

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
                {4, 4.0},
                {5, 5.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[2]));
  ASSERT_EQ((SampleList{
                {1, 6.0},
                {2, 7.0},
                {3, 8.0},
                {4, 9.0},
                {5, 10.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[3]));
}

class LoaderUnorderedVectorTestFixture : public ::testing::Test {
 protected:
  Loader::UnorderedVector vector_;
};

TEST_F(LoaderUnorderedVectorTestFixture, Empty) {
  // Arrange

  // Act

  // Assert
  ASSERT_TRUE(vector_.empty());
  ASSERT_EQ(vector_.size(), 0);
  ASSERT_EQ(std::distance(vector_.begin(), vector_.end()), 0);
}

TEST_F(LoaderUnorderedVectorTestFixture, NotEmpty) {
  // Arrange

  // Act
  vector_.insert(1);

  // Assert
  ASSERT_FALSE(vector_.empty());
  ASSERT_EQ(vector_.size(), 1);
  ASSERT_EQ(std::distance(vector_.begin(), vector_.end()), 1);
}

TEST_F(LoaderUnorderedVectorTestFixture, ReserveEmpty) {
  // Arrange
  vector_.reserve(100);

  // Act

  // Assert
  ASSERT_TRUE(vector_.empty());
  ASSERT_EQ(vector_.size(), 0);
  ASSERT_EQ(std::distance(vector_.begin(), vector_.end()), 0);
}

TEST_F(LoaderUnorderedVectorTestFixture, ReserveInsert) {
  // Arrange
  vector_.reserve(10);

  // Act
  vector_.insert(1);
  vector_.insert(10);
  vector_.insert(100);

  // Assert
  ASSERT_EQ(vector_.size(), 3);

  ASSERT_NE(vector_.find(1), vector_.end());
  ASSERT_NE(vector_.find(10), vector_.end());
  ASSERT_NE(vector_.find(100), vector_.end());

  ASSERT_EQ(vector_.find(13), vector_.end());
}

TEST_F(LoaderUnorderedVectorTestFixture, NoReserveInsert) {
  // Arrange

  // Act
  vector_.insert(1);
  vector_.insert(10);
  vector_.insert(100);

  // Assert
  ASSERT_EQ(vector_.size(), 3);

  ASSERT_NE(vector_.find(1), vector_.end());
  ASSERT_NE(vector_.find(10), vector_.end());
  ASSERT_NE(vector_.find(100), vector_.end());

  ASSERT_EQ(vector_.find(13), vector_.end());
}

TEST_F(LoaderUnorderedVectorTestFixture, ModifyInfoAndFind) {
  // Arrange
  auto [ls_id, info] = *vector_.insert(100);
  info.chunk_id = 10;
  info.buffer.push_back_u64(0xAABBCCDD);

  // Act
  const auto it = vector_.find(100);

  // Assert
  ASSERT_EQ((*it).first, 100);
  ASSERT_EQ((*it).second.chunk_id, 10);
  ASSERT_EQ((*it).second.buffer.reader().consume_u64(), 0xAABBCCDD);
}

TEST_F(LoaderUnorderedVectorTestFixture, ModifyInfoAndClear) {
  // Arrange
  vector_.insert(1);
  vector_.insert(10);
  vector_.insert(100);

  // Act
  vector_.clear();

  // Assert
  ASSERT_TRUE(vector_.empty());
  ASSERT_EQ(vector_.size(), 0);
  ASSERT_EQ(std::distance(vector_.begin(), vector_.end()), 0);
}

TEST_F(LoaderUnorderedVectorTestFixture, ModifyInfoAndClearAndInsert) {
  // Arrange
  auto [ls_id, info] = *vector_.insert(100);
  info.chunk_id = 10;
  info.buffer.push_back_u64(0xAABBCCDD);
  vector_.clear();

  // Act
  const auto it = vector_.insert(100);

  // Assert
  ASSERT_EQ((*it).first, 100);
  ASSERT_EQ((*it).second.chunk_id, 0);
  ASSERT_EQ((*it).second.buffer.size_in_bits(), 0);
}

}  // namespace