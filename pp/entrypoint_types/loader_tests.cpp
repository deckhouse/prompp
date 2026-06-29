#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "entrypoint_types/loader.h"
#include "primitives/label_set.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/encoder/sample.h"
#include "series_data/unloading/unloader.h"

namespace {

using entrypoint_types::QueryableEncodingBimap;
using PromPP::Primitives::LabelViewSet;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::encoder::SampleList;
using series_data::unloading::Unloader;

class RevertableLoaderFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  Unloader unloader_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
  QueryableEncodingBimap lss_;

  void SetUp() override {
    encoder_.encode(0, 1, 1.0);
    encoder_.encode(0, 2, 2.0);
    encoder_.encode(0, 3, 3.0);
    encoder_.encode(0, 4, 4.0);
    encoder_.encode(0, 5, 5.0);

    lss_.find_or_emplace(LabelViewSet{{"job", "a"}});
    lss_.build_deferred_indexes();
  }

  void encode_more(uint32_t ls_id, const LabelViewSet& label_set, const SampleList& samples) {
    for (const auto& sample : samples) {
      encoder_.encode(ls_id, sample.timestamp, sample.value);
    }

    lss_.find_or_emplace(label_set);
    lss_.build_deferred_indexes();
  }

  [[nodiscard]] auto open_chunk_stream(uint32_t ls_id) const {
    return storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[ls_id].encoder.external_index);
  }

  [[nodiscard]] SampleList decode_open_chunk(uint32_t ls_id) const {
    return Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[ls_id]);
  }
};

TEST_F(RevertableLoaderFixture, LoadFinalizeRestoresUnloadedOpenChunk) {
  // Arrange
  unloader_.create_snapshot(stream_);
  unloader_.unload();

  // Act
  entrypoint_types::RevertableLoader loader{storage_, lss_.ls_id_set().begin(), lss_.ls_id_set().end(), 1};
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  // Assert
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decode_open_chunk(0));
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(RevertableLoaderFixture, RevertRestoresUnloadedOpenChunk) {
  // Arrange
  unloader_.create_snapshot(stream_);
  unloader_.unload();
  const auto trimmed_stream = open_chunk_stream(0);

  entrypoint_types::RevertableLoader loader{storage_, lss_.ls_id_set().begin(), lss_.ls_id_set().end(), 1};
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  // Act
  loader.revert();
  const auto restored_stream = open_chunk_stream(0);

  // Assert
  EXPECT_EQ(trimmed_stream, restored_stream);
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(RevertableLoaderFixture, LoadFinalizeLoadsSeriesByBatch) {
  // Arrange
  encode_more(1, LabelViewSet{{"job", "b"}}, SampleList{{1, 11.0}, {2, 12.0}, {3, 13.0}, {4, 14.0}, {5, 15.0}});
  encode_more(2, LabelViewSet{{"job", "c"}}, SampleList{{1, 21.0}, {2, 22.0}, {3, 23.0}, {4, 24.0}, {5, 25.0}});

  unloader_.create_snapshot(stream_);
  unloader_.unload();

  entrypoint_types::RevertableLoader loader{storage_, lss_.ls_id_set().begin(), lss_.ls_id_set().end(), 2};

  // Act
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  const auto has_second_batch = loader.next_batch();
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  const auto has_third_batch = loader.next_batch();

  // Assert
  EXPECT_TRUE(has_second_batch);
  EXPECT_FALSE(has_third_batch);

  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(1));
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(2));

  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decode_open_chunk(0));
  EXPECT_EQ((SampleList{{1, 11.0}, {2, 12.0}, {3, 13.0}, {4, 14.0}, {5, 15.0}}), decode_open_chunk(1));
  EXPECT_EQ((SampleList{{1, 21.0}, {2, 22.0}, {3, 23.0}, {4, 24.0}, {5, 25.0}}), decode_open_chunk(2));
}

TEST_F(RevertableLoaderFixture, RevertRestoresSeriesLoadedAcrossBatches) {
  // Arrange
  encode_more(1, LabelViewSet{{"job", "b"}}, SampleList{{1, 11.0}, {2, 12.0}, {3, 13.0}, {4, 14.0}, {5, 15.0}});
  encode_more(2, LabelViewSet{{"job", "c"}}, SampleList{{1, 21.0}, {2, 22.0}, {3, 23.0}, {4, 24.0}, {5, 25.0}});

  unloader_.create_snapshot(stream_);
  unloader_.unload();
  const auto trimmed_stream0 = open_chunk_stream(0);
  const auto trimmed_stream1 = open_chunk_stream(1);
  const auto trimmed_stream2 = open_chunk_stream(2);

  entrypoint_types::RevertableLoader loader{storage_, lss_.ls_id_set().begin(), lss_.ls_id_set().end(), 2};

  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  const auto has_second_batch = loader.next_batch();
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  // Act
  loader.revert();
  const auto restored_stream0 = open_chunk_stream(0);
  const auto restored_stream1 = open_chunk_stream(1);
  const auto restored_stream2 = open_chunk_stream(2);

  // Assert
  ASSERT_TRUE(has_second_batch);

  EXPECT_EQ(trimmed_stream0, restored_stream0);
  EXPECT_EQ(trimmed_stream1, restored_stream1);
  EXPECT_EQ(trimmed_stream2, restored_stream2);

  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(1));
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(2));
}

}  // namespace
