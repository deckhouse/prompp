#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/unloading/unloader.h"

#include "series_data/unloading/loader.h"

namespace {

using namespace series_data;
using namespace series_data::unloading;

class UnloaderTrait {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
  Unloader unloader_{storage_};

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> get_buffer() const noexcept {
    return {reinterpret_cast<const uint8_t*>(stream_.view().data()), stream_.view().size()};
  }

  void mark_series_as_unused(uint32_t ls_id) { storage_.unused_series_bitmap.add(ls_id); }
};

class UnloaderTestFixture : public UnloaderTrait, public testing::Test {
 protected:
  void SetUp() override {
    storage_.reset();
    stream_.clear();
  }
};

TEST_F(UnloaderTestFixture, EmptyUnloader) {
  // Arrange

  // Act
  unloader_.unload(stream_);

  // Assert
  ASSERT_EQ(stream_.view().size(), unloader_.get_empty_unloader_size_in_bytes());
}

TEST_F(UnloaderTestFixture, UnloadOneChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  mark_series_as_unused(0);

  const uint32_t chunk_stream_size_in_bits =
      storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits();

  // Act
  unloader_.unload(stream_);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);
}

TEST_F(UnloaderTestFixture, LoadOneChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  encoder_.encode(15, 1, 1.0);
  encoder_.encode(15, 2, 2.0);
  encoder_.encode(15, 3, 3.0);
  encoder_.encode(15, 4, 4.0);
  encoder_.encode(15, 5, 5.0);

  mark_series_as_unused(0);
  mark_series_as_unused(15);

  const uint32_t chunk_stream_size_in_bits =
      storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits();

  // Act
  unloader_.unload(stream_);

  auto span = stream_.span<uint8_t>();
  std::vector<uint32_t> chunk_ids = {0, 15};
  Loader loader(storage_, chunk_ids);
  loader.load_next(span);

  // Assert
  ASSERT_EQ(storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(storage_.open_chunks[0].encoder.external_index).size_in_bits(),
            chunk_stream_size_in_bits % 8);
}

}  // namespace