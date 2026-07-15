#include <gtest/gtest.h>

#include "series_data/encoder.h"
#include "series_data/querier/instant_querier.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Timestamp;
using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::InstantQuerier;
using series_data::chunk::DataChunk;
using series_data::encoder::Sample;

struct InstantQuerierRequest {
  Timestamp timestamp{};
  LabelSetID ls_id{};
};

struct InstantQuerierCase {
  InstantQuerierRequest request{};
  Sample expected_sample{};
  bool expect_queried{};
};

class InstantQuerierOpenChunkFixture : public testing::TestWithParam<InstantQuerierCase> {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  Sample default_sample_ = {.timestamp = -1, .value = STALE_NAN};
  std::vector<Sample> samples_{default_sample_};
};

TEST_F(InstantQuerierOpenChunkFixture, EmptyChunk) {
  // Arrange
  static constexpr auto kEmptyChunkLsId = 0U;
  std::vector<uint32_t> ls_ids = {kEmptyChunkLsId};

  encoder_.encode(1, 1, 1.0);

  // Act
  InstantQuerier{storage_}.query(samples_, ls_ids, 0);

  // Assert
  EXPECT_EQ(default_sample_, samples_[0]);
  EXPECT_FALSE(storage_.queried_series_bitmap.is_set(0));
}

TEST_P(InstantQuerierOpenChunkFixture, InstantQueryOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  // Act
  InstantQuerier{storage_}.query(samples_, std::vector{GetParam().request.ls_id}, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, samples_[0]);
  EXPECT_EQ(storage_.queried_series_bitmap.is_set(0), GetParam().expect_queried);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeOpenChunk,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkFirstPoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 1, .value = 1.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkMiddlePoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 3, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 3, .value = 3.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkLastPoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 5, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 5, .value = 5.0},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickAfterOpenChunk,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 5, .value = 5.0},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

class InstantQuerierFinalizedChunkFixture : public InstantQuerierOpenChunkFixture {};

TEST_P(InstantQuerierFinalizedChunkFixture, InstantQueryFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 10, 10.0);

  // Act
  InstantQuerier{storage_}.query(samples_, std::vector{GetParam().request.ls_id}, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, samples_[0]);
  EXPECT_EQ(storage_.queried_series_bitmap.is_set(0), GetParam().expect_queried);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeFinalizedChunk,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickOpenFinalizedFirstPoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 1, .value = 1.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickFinalizedChunkMiddlePoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 3, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 3, .value = 3.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickFinalizedChunkLastPoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 5, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 5, .value = 5.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickAfterFinalizedChunk,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 5, .value = 5.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

class InstantQuerierOpenAndFinalizedChunkFixture : public InstantQuerierOpenChunkFixture {};

TEST_P(InstantQuerierOpenAndFinalizedChunkFixture, InstantQueryFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 10, 10.0);
  encoder_.encode(0, 11, 11.0);
  encoder_.encode(0, 12, 12.0);
  encoder_.encode(0, 13, 13.0);
  encoder_.encode(0, 14, 14.0);

  // Act
  InstantQuerier{storage_}.query(samples_, std::vector{GetParam().request.ls_id}, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, samples_[0]);
  EXPECT_EQ(storage_.queried_series_bitmap.is_set(0), GetParam().expect_queried);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeSeries,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickSeriesFirstPoint,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 1, .value = 1.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickSeriesMiddlePointInOpenChunk,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 4, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 4, .value = 4.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickSeriesMiddlePointInFinalizedChunk,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 11, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 11, .value = 11.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickSeriesLastPoint,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 14, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 14, .value = 14.0},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickAfterSeries,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 20, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 14, .value = 14.0},
                                                            .expect_queried = false}));

INSTANTIATE_TEST_SUITE_P(PickBetweenFinalizedAndOpen,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 8, .ls_id = 0},
                                                            .expected_sample = Sample{.timestamp = 5, .value = 5.0},
                                                            .expect_queried = true}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                                                            .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN},
                                                            .expect_queried = false}));

class InstantQuerierLoaderUnloaderTestFixture : public testing::Test {
 protected:
  void SetUp() override {
    for (uint32_t ls_id = 0; ls_id < 5; ++ls_id) {
      encoder_.encode(ls_id, 1, get_value(ls_id, 1));
      encoder_.encode(ls_id, 2, get_value(ls_id, 2));
      encoder_.encode(ls_id, 3, get_value(ls_id, 3));
      encoder_.encode(ls_id, 4, get_value(ls_id, 4));
      encoder_.encode(ls_id, 5, get_value(ls_id, 5));
    }
  }

  static double get_value(uint32_t ls_id, int64_t timestamp) noexcept { return static_cast<double>(10ll * ls_id + timestamp); }

  DataStorage storage_;
  Encoder<> encoder_{storage_};
  InstantQuerier instant_querier_{storage_};
  const Sample default_sample_{.timestamp = -1, .value = STALE_NAN};
  std::vector<Sample> samples_{3, default_sample_};
};

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryNeedLoading) {
  // Arrange
  storage_.queried_series_bitmap.set({0, 2, 4});
  storage_.unloaded_series_bitmap.set({1, 3});

  // Act
  instant_querier_.query(samples_, std::initializer_list{0, 1, 2}, 3);

  // Assert
  ASSERT_TRUE(instant_querier_.need_loading());

  ASSERT_TRUE(std::ranges::equal(instant_querier_.get_series_to_load(), std::initializer_list{1}));

  ASSERT_EQ((std::vector{Sample{.timestamp = 3, .value = get_value(0, 3)}, default_sample_, Sample{.timestamp = 3, .value = get_value(2, 3)}}), samples_);
}

TEST_F(InstantQuerierLoaderUnloaderTestFixture, InstantQueryLoading) {
  // Arrange
  storage_.queried_series_bitmap.set({0, 2, 4});
  storage_.unloaded_series_bitmap.set({1, 3});

  instant_querier_.query(samples_, std::initializer_list{0, 1, 2}, 4);

  storage_.unloaded_series_bitmap.clear();

  // Act
  instant_querier_.query_reload(samples_, std::initializer_list{0, 1, 2}, 4);

  // Assert
  ASSERT_TRUE(std::ranges::equal(storage_.queried_series_bitmap, std::initializer_list{0, 1, 2, 4}));

  ASSERT_EQ((std::vector{Sample{.timestamp = 4, .value = get_value(0u, 4)}, Sample{.timestamp = 4, .value = get_value(1u, 4)},
                         Sample{.timestamp = 4, .value = get_value(2, 4)}}),
            samples_);
}

}  // namespace