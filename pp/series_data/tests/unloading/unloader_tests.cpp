#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

namespace {

using series_data::Decoder;
using series_data::chunk::DataChunk;
using series_data::unloading::Unloader;
using std::operator""sv;

class UnloaderFixture : public ::testing::Test {
 protected:
  static constexpr auto kEmptySnapshot = "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"sv;

  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  Unloader unloader_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
};

TEST_F(UnloaderFixture, EmptyDataStorage) {
  // Arrange

  // Act
  unloader_.create_snapshot(stream_);
  unloader_.unload();

  // Assert
  EXPECT_EQ(stream_.view(), kEmptySnapshot);
  EXPECT_EQ(0U, storage_.unloaded_series_bitmap.popcount());
}

TEST_F(UnloaderFixture, NoUnloadableSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(1, 1, -1.0);

  encoder_.encode(2, 1, 1.1);

  encoder_.encode(3, 1, 1.0);
  encoder_.encode(3, 1, 1.1);

  // Act
  unloader_.create_snapshot(stream_);
  unloader_.unload();

  // Assert
  EXPECT_EQ(stream_.view(), kEmptySnapshot);
  EXPECT_EQ(0U, storage_.unloaded_series_bitmap.popcount());
}

TEST_F(UnloaderFixture, DontUnloadQueriedSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  storage_.queried_series_bitmap.set(0);

  // Act
  unloader_.create_snapshot(stream_);
  unloader_.unload();

  // Assert
  EXPECT_EQ(stream_.view(), kEmptySnapshot);
  EXPECT_EQ(0U, storage_.unloaded_series_bitmap.popcount());
}

TEST_F(UnloaderFixture, DontUnloadQueriedSeriesAfterCreateSnapshot) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  const auto& encoder_stream = storage_.get_asc_integer_stream<DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index);
  const uint32_t chunk_stream_size = encoder_stream.size_in_bits();

  // Act
  unloader_.create_snapshot(stream_);
  storage_.queried_series_bitmap.set(0);
  unloader_.unload();

  // Assert
  EXPECT_EQ(chunk_stream_size, encoder_stream.size_in_bits());
  EXPECT_EQ(0U, storage_.unloaded_series_bitmap.popcount());
}

}  // namespace