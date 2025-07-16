#include <gtest/gtest.h>

#include "bare_bones/streams.h"
#include "primitives/sample.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"

namespace {
using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::InstantQuerier;
using series_data::encoder::Sample;

class InstantQuerierLoaderUnloaderTrait {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  InstantQuerier instant_querier_{storage_};
  std::vector<Sample> samples_{5, Sample{.timestamp = 0, .value = STALE_NAN}};
  std::vector<uint32_t> indices_unloaded_{0, 2, 4, 6, 8};
  std::vector<uint32_t> indices_queried_{0, 1, 2, 3, 4};
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

  void query_mock(const std::vector<uint32_t>& ld_ids) {
    for (uint32_t ls_id : ld_ids) {
      storage_.queried_series_bitmap.set(ls_id);
    }
  }

  void unload_mock() {
    for (uint32_t ls_id = 0; ls_id < 10; ++ls_id) {
      if (!storage_.queried_series_bitmap.is_set(ls_id)) {
        storage_.unloaded_series_bitmap.set(ls_id);
      }
    }
  }

  void load_mock() { storage_.unloaded_series_bitmap.clear(); }
};

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryNeedLoading) {
  // Arrange
  query_mock(indices_unloaded_);
  unload_mock();

  // Act
  instant_querier_.query(samples_, indices_queried_, 3);

  // Assert
  ASSERT_TRUE(instant_querier_.need_loading());

  ASSERT_EQ(instant_querier_.get_series_to_load().popcount(), 2);
  ASSERT_TRUE(instant_querier_.get_series_to_load().is_set(1));
  ASSERT_TRUE(instant_querier_.get_series_to_load().is_set(3));

  ASSERT_EQ((std::vector<Sample>{{.timestamp = 3, .value = 2.0},
                                 {.timestamp = 0, .value = STALE_NAN},
                                 {.timestamp = 3, .value = 12.0},
                                 {.timestamp = 0, .value = STALE_NAN},
                                 {.timestamp = 3, .value = 22.0}}),
            samples_);
}

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryLoading) {
  // Arrange
  query_mock(indices_unloaded_);
  unload_mock();
  instant_querier_.query(samples_, indices_queried_, 4);
  load_mock();

  // Act
  instant_querier_.query_reload(samples_, indices_queried_, 4);

  // Assert
  ASSERT_EQ(storage_.queried_series_bitmap.popcount(), 7);

  ASSERT_EQ((std::vector<Sample>{{.timestamp = 4, .value = 3.0},
                                 {.timestamp = 4, .value = 8.0},
                                 {.timestamp = 4, .value = 13.0},
                                 {.timestamp = 4, .value = 18.0},
                                 {.timestamp = 4, .value = 23.0}}),
            samples_);
}

using Query = series_data::querier::Query<>;

class QuerierLoaderUnloaderTrait {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  series_data::querier::Querier querier_{storage_};

  PromPP::Primitives::TimeInterval interval_{.min = 1, .max = 3};

  Query query1_{.time_interval = interval_, .label_set_ids = {0, 2, 4}};
  Query query2_{.time_interval = interval_, .label_set_ids = {1, 2, 3}};
};

class QuerierLoaderUnloaderTestFixture : public QuerierLoaderUnloaderTrait, public ::testing::Test {
 protected:
  void SetUp() override {
    uint32_t idx = 0;
    for (uint32_t ls_id = 0; ls_id < 5; ++ls_id) {
      encoder_.encode(ls_id, 1, idx++);
      encoder_.encode(ls_id, 2, idx++);
      encoder_.encode(ls_id, 3, idx++);
      encoder_.encode(ls_id, 4, idx++);
      encoder_.encode(ls_id, 5, idx++);
    }
  }

  void query_mock() {
    for (uint32_t ls_id : query1_.label_set_ids) {
      storage_.queried_series_bitmap.set(ls_id);
    }
  }

  void unload_mock() {
    for (uint32_t ls_id = 0; ls_id < 5; ++ls_id) {
      if (!storage_.queried_series_bitmap.is_set(ls_id)) {
        storage_.unloaded_series_bitmap.set(ls_id);
      }
    }
  }

  void load_mock() { storage_.unloaded_series_bitmap.clear(); }
};

TEST_F(QuerierLoaderUnloaderTestFixture, QuerierNeedLoading) {
  // Arrange
  query_mock();
  unload_mock();

  // Act
  std::ignore = querier_.query(query2_);

  // Assert
  EXPECT_TRUE(querier_.need_loading());

  EXPECT_EQ(querier_.get_series_to_load().popcount(), 2);
  EXPECT_TRUE(querier_.get_series_to_load().is_set(1));
  EXPECT_TRUE(querier_.get_series_to_load().is_set(3));
}
}  // namespace