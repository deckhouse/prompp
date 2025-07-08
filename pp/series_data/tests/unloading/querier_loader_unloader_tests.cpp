#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "primitives/sample.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

class InstantQuerierLoaderUnloaderTrait {
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

class InstantQuerierLoaderUnloaderTestFixture : public InstantQuerierLoaderUnloaderTrait, public testing::Test {
 protected:
  void SetUp() override {
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

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryNeedLoading) {
  // Arrange

  // Act
  instant_querier_.query(samples_, indices_first_, 2);

  unloader_.unload(stream_);

  instant_querier_.query(samples_, indices_second_, 3);

  // Assert
  ASSERT_EQ(storage_.queried_series_bitmap.popcount(), 5);

  ASSERT_TRUE(instant_querier_.need_loading());

  ASSERT_EQ(instant_querier_.get_series_to_load().popcount(), 2);
  ASSERT_TRUE(instant_querier_.get_series_to_load().is_set(1));
  ASSERT_TRUE(instant_querier_.get_series_to_load().is_set(3));

  ASSERT_EQ(samples_[0], series_data::encoder::Sample(3, 2.0));
  ASSERT_EQ(samples_[1].timestamp, 2);
  ASSERT_EQ(samples_[2], series_data::encoder::Sample(3, 12.0));
  ASSERT_EQ(samples_[3].timestamp, 2);
  ASSERT_EQ(samples_[4], series_data::encoder::Sample(3, 22.0));

  ASSERT_EQ(storage_.unloaded_series_bitmap.popcount(), 5);

  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(1));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(3));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(5));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(7));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(9));
}

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryLoading) {
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
  ASSERT_EQ(storage_.queried_series_bitmap.popcount(), 7);

  ASSERT_EQ(samples_[0], series_data::encoder::Sample(4, 3.0));
  ASSERT_EQ(samples_[1], series_data::encoder::Sample(4, 8.0));
  ASSERT_EQ(samples_[2], series_data::encoder::Sample(4, 13.0));
  ASSERT_EQ(samples_[3], series_data::encoder::Sample(4, 18.0));
  ASSERT_EQ(samples_[4], series_data::encoder::Sample(4, 23.0));

  ASSERT_EQ(storage_.unloaded_series_bitmap.popcount(), 3);

  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(5));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(7));
  ASSERT_TRUE(storage_.unloaded_series_bitmap.is_set(9));
}

using Query = series_data::querier::Query<BareBones::Vector<PromPP::Primitives::LabelSetID>>;

class QuerierLoaderUnloaderTrait {
 protected:
  series_data::DataStorage storage_;
  series_data::Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream stream_;
  series_data::unloading::Unloader unloader_{storage_};
  series_data::querier::Querier querier_{storage_};

  PromPP::Primitives::TimeInterval interval_{.min = 1, .max = 3};
  PromPP::Primitives::TimeInterval reload_interval_{.min = 1, .max = 5};
};

class QuerierLoaderUnloaderTestFixture : public QuerierLoaderUnloaderTrait, public ::testing::Test {
 protected:
  void SetUp() override {
    storage_.reset();
    stream_.clear();

    uint32_t idx = 0;
    for (uint32_t ls_id = 0; ls_id < 5; ++ls_id) {
      encoder_.encode(ls_id, 1, idx++);
      encoder_.encode(ls_id, 2, idx++);
      encoder_.encode(ls_id, 3, idx++);
      encoder_.encode(ls_id, 4, idx++);
      encoder_.encode(ls_id, 5, idx++);
    }
  }
};

TEST_F(QuerierLoaderUnloaderTestFixture, QuerierNeedLoading) {
  // Arrange
  Query query1{.time_interval = interval_, .label_set_ids = {0, 2, 4}};
  Query query2{.time_interval = interval_, .label_set_ids = {1, 2, 3}};

  // Act
  std::ignore = querier_.query(query1);

  unloader_.unload(stream_);

  std::ignore = querier_.query(query2);

  // Assert
  EXPECT_TRUE(querier_.need_loading());

  EXPECT_EQ(querier_.get_series_to_load().popcount(), 2);
  EXPECT_TRUE(querier_.get_series_to_load().is_set(1));
  EXPECT_TRUE(querier_.get_series_to_load().is_set(3));

  EXPECT_EQ(storage_.queried_series_bitmap.popcount(), 5);

  EXPECT_EQ(storage_.unloaded_series_bitmap.popcount(), 2);

  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(1));
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(3));
}

TEST_F(QuerierLoaderUnloaderTestFixture, QuerierLoad) {
  // Arrange
  Query query1{.time_interval = interval_, .label_set_ids = {0, 2, 4}};
  Query query2{.time_interval = reload_interval_, .label_set_ids = {0, 1, 2}};

  // Act
  std::ignore = querier_.query(query1);

  unloader_.unload(stream_);

  std::ignore = querier_.query(query2);

  series_data::unloading::Loader loader(storage_, querier_);
  loader.load_next(stream_.span<const uint8_t>());
  loader.load_finalize();

  // Assert
  EXPECT_EQ(storage_.queried_series_bitmap.popcount(), 4);

  EXPECT_EQ(storage_.unloaded_series_bitmap.popcount(), 1);
  EXPECT_TRUE(storage_.unloaded_series_bitmap.is_set(3));
}