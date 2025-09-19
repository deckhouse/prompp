#include <gtest/gtest.h>

#include "series_data/unloading/loader.h"
#include "series_data/unloading/reverter.h"
#include "series_data/unloading/unloader.h"

namespace {

using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::chunk::DataChunk;
using series_data::encoder::SampleList;
using series_data::unloading::Loader;
using series_data::unloading::LoadReverter;
using series_data::unloading::Unloader;

class ReverterTestFixture : public testing::Test {
 protected:
  DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  Unloader unloader_{storage_};
  LoadReverter reverter_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;

  template <class... Spans>
  void load(const std::vector<uint32_t>& ls_ids, Spans&&... spans) {
    Loader loader(storage_, ls_ids, ls_ids.size());
    (..., loader.load_next(std::forward<Spans>(spans)));
    loader.load_finalize();
  }

  void unload() {
    unloader_.create_snapshot(stream_);
    unloader_.unload();
  }
};

TEST_F(ReverterTestFixture, RevertOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unload();

  const auto chunk_stream_trimmed = storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index);

  reverter_.add_series_to_revert(std::initializer_list{0u}, 1);

  load({0}, stream_.span<const uint8_t>());

  // Act
  reverter_.revert();

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index), chunk_stream_trimmed);
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(ReverterTestFixture, RevertFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  unload();

  const auto chunk_stream_trimmed = storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index);

  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 6, 6.0);

  reverter_.add_series_to_revert(std::initializer_list{0u}, 1);

  load({0}, stream_.span<const uint8_t>());

  // Act
  reverter_.revert();

  // Assert
  ASSERT_EQ(storage_.finalized_data_streams.at(0), chunk_stream_trimmed);
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(ReverterTestFixture, NoRevertOutdatedSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(0, 0, 0.0);

  const auto chunk_stream_original = storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index);

  unload();

  reverter_.add_series_to_revert(std::initializer_list{0u}, 1);

  load({0}, stream_.span<const uint8_t>());

  // Act
  reverter_.revert();

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
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(ReverterTestFixture, NoRevertQueriedSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload();
  reverter_.add_series_to_revert(std::initializer_list{0u}, 1);
  load({0}, stream_.span<const uint8_t>());

  // Act
  storage_.queried_series_bitmap.set(0);
  reverter_.revert();

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(ReverterTestFixture, NoRevertUnmodifiedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  reverter_.add_series_to_revert(std::initializer_list{0u}, 1);

  // Act
  reverter_.revert();

  // Assert
  ASSERT_EQ((SampleList{
                {1, 1.0},
                {2, 2.0},
                {3, 3.0},
            }),
            Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]));
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

}  // namespace