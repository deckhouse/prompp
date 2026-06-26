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

  [[nodiscard]] auto open_chunk_stream() const {
    return storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index);
  }

  [[nodiscard]] SampleList decode_open_chunk() const { return Decoder::decode_chunk<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0]); }
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
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decode_open_chunk());
  EXPECT_FALSE(storage_.unloaded_series_bitmap.is_set(0));
}

TEST_F(RevertableLoaderFixture, RevertRestoresUnloadedOpenChunk) {
  // Arrange
  unloader_.create_snapshot(stream_);
  unloader_.unload();
  const auto trimmed_stream = open_chunk_stream();

  entrypoint_types::RevertableLoader loader{storage_, lss_.ls_id_set().begin(), lss_.ls_id_set().end(), 1};
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  // Act
  loader.revert();
  const auto restored_stream = open_chunk_stream();

  // Assert
  EXPECT_EQ(trimmed_stream, restored_stream);
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(0));
}

}  // namespace
