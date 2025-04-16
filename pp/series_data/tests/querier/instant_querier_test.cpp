#include <gtest/gtest.h>

#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "series_data/querier/instant_querier.h"

namespace {
using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Timestamp;
using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::OutdatedSampleEncoder;
using series_data::chunk::DataChunk;
using series_data::encoder::Sample;

struct InstantQuerierRequest {
  Timestamp timestamp{};
  LabelSetID ls_id{};
};

struct InstantQuerierCase {
  InstantQuerierRequest request{};
  Sample expected_sample{};
};

class InstantQuerierOpenChunkFixture : public testing::TestWithParam<InstantQuerierCase> {
protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  Sample sample{.timestamp = -1, .value = STALE_NAN};
};

TEST_P(InstantQuerierOpenChunkFixture, InstantQueryOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, GetParam().request.ls_id, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, sample);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeOpenChunk,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkFirstPoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 1, .value = 1.0}}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkMiddlePoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 3, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 3, .value = 3.0}}));

INSTANTIATE_TEST_SUITE_P(PickOpenChunkLastPoint,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 5, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 5, .value = 5.0}}));

INSTANTIATE_TEST_SUITE_P(PickAfterOpenChunk,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 5, .value = 5.0}}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierOpenChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

class InstantQuerierFinalizedChunkFixture : public InstantQuerierOpenChunkFixture {
};

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
  series_data::InstantQuerier::query_sample(sample, storage_, GetParam().request.ls_id, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, sample);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeFinalizedChunk,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(PickOpenFinalizedFirstPoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 1, .value = 1.0}}));

INSTANTIATE_TEST_SUITE_P(PickFinalizedChunkMiddlePoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 3, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 3, .value = 3.0}}));

INSTANTIATE_TEST_SUITE_P(PickFinalizedChunkLastPoint,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 5, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 5, .value = 5.0}}));

INSTANTIATE_TEST_SUITE_P(PickAfterFinalizedChunk,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 5, .value = 5.0}}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

class InstantQuerierOpenAndFinalizedChunkFixture : public InstantQuerierOpenChunkFixture {
};

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
  series_data::InstantQuerier::query_sample(sample, storage_, GetParam().request.ls_id, GetParam().request.timestamp);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, sample);
}

INSTANTIATE_TEST_SUITE_P(PickBeforeSeries,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 0, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(PickSeriesFirstPoint,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 1, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 1, .value = 1.0}}));

INSTANTIATE_TEST_SUITE_P(PickSeriesMiddlePointInOpenChunk,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 4, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 4, .value = 4.0}}));

INSTANTIATE_TEST_SUITE_P(PickSeriesMiddlePointInFinalizedChunk,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 11, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 11, .value = 11.0}}));

INSTANTIATE_TEST_SUITE_P(PickSeriesLastPoint,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 14, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 14, .value = 14.0}}));

INSTANTIATE_TEST_SUITE_P(PickAfterSeries,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 20, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 14, .value = 14.0}}));

INSTANTIATE_TEST_SUITE_P(PickBetweenFinalizedAndOpen,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 8, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 5, .value = 5.0}}));

INSTANTIATE_TEST_SUITE_P(PickNonExistingLsID,
                         InstantQuerierOpenAndFinalizedChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.timestamp = 6, .ls_id = 1},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));
} // namespace