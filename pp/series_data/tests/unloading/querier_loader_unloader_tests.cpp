#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "primitives/sample.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

class QuerierLoaderUnloaderTrait {
 protected:
  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
  series_data::unloading::Unloader unloader_{storage_};
  series_data::InstantQuerier instant_querier_{storage_};
  std::vector<series_data::encoder::Sample> samples_{5};
  std::vector<uint32_t> indices_first_{0, 2, 4, 6, 8};
  std::vector<uint32_t> indices_second_{0, 1, 2, 3, 4};
};

class QuerierLoaderUnloaderTestFixture : public QuerierLoaderUnloaderTrait, public testing::Test {
 protected:
  void SetUp() override {
    storage_.reset();
    stream_.clear();

    uint32_t idx = 0;
    for (uint32_t ls_id = 0; ls_id < 10; ++ls_id) {
      encoder_.encode(ls_id, 1, idx++);
      encoder_.encode(ls_id, 2, idx++);
      encoder_.encode(ls_id, 3, idx++);
      encoder_.encode(ls_id, 4, idx++);
      encoder_.encode(ls_id, 5, idx++);
    }
  }
};

TEST_F(QuerierLoaderUnloaderTestFixture, InstantQueryNeedLoading) {
  // Arrange

  // Act
  instant_querier_.query(samples_, indices_first_, 2);

  unloader_.unload(stream_);

  instant_querier_.query(samples_, indices_second_, 3);

  // Assert
  ASSERT_EQ(storage_.queried_series_bitmap.cardinality(), 5);

  ASSERT_TRUE(instant_querier_.need_loading());

  ASSERT_EQ(instant_querier_.get_series_to_load().cardinality(), 2);
  ASSERT_TRUE(instant_querier_.get_series_to_load().contains(1));
  ASSERT_TRUE(instant_querier_.get_series_to_load().contains(3));

  ASSERT_EQ(samples_[0], series_data::encoder::Sample(3, 2.0));
  ASSERT_EQ(samples_[1].timestamp, 2);
  ASSERT_EQ(samples_[2], series_data::encoder::Sample(3, 12.0));
  ASSERT_EQ(samples_[3].timestamp, 2);
  ASSERT_EQ(samples_[4], series_data::encoder::Sample(3, 22.0));

  ASSERT_EQ(storage_.unloaded_series_bitmap.cardinality(), 5);

  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(1));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(3));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(5));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(7));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(9));
}

TEST_F(QuerierLoaderUnloaderTestFixture, InstantQueryLoading) {
  // Arrange

  // Act
  instant_querier_.query(samples_, indices_first_, 2);

  unloader_.unload(stream_);

  instant_querier_.query(samples_, indices_second_, 4);

  series_data::unloading::Loader loader(storage_, instant_querier_);
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  instant_querier_.query_reload(samples_, indices_second_, 4);

  // Assert
  ASSERT_EQ(storage_.queried_series_bitmap.cardinality(), 7);

  ASSERT_EQ(samples_[0], series_data::encoder::Sample(4, 3.0));
  ASSERT_EQ(samples_[1], series_data::encoder::Sample(4, 8.0));
  ASSERT_EQ(samples_[2], series_data::encoder::Sample(4, 13.0));
  ASSERT_EQ(samples_[3], series_data::encoder::Sample(4, 18.0));
  ASSERT_EQ(samples_[4], series_data::encoder::Sample(4, 23.0));

  ASSERT_EQ(storage_.unloaded_series_bitmap.cardinality(), 3);

  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(5));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(7));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.contains(9));
}
