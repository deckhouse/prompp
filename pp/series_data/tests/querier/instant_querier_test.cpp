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
using PromPP::Primitives::TimeInterval;

struct InstantQuerierRequest {
  TimeInterval time_interval{};
  LabelSetID ls_id{};
};

struct InstantQuerierCase {
  InstantQuerierRequest request{};
  Sample expected_sample{};
};

class InstantQuerierFixture : public testing::TestWithParam<InstantQuerierCase> {
protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};

  void fill_all() {
    encoder_.encode(0, 100, 1.0);
    encoder_.encode(0, 101, STALE_NAN);

    encoder_.encode(1, 102, 1.1);
    encoder_.encode(1, 103, 1.1);

    encoder_.encode(2, 104, 1.1);
    encoder_.encode(2, 105, 1.2);
    encoder_.encode(2, 106, 1.3);

    encoder_.encode(3, 107, 1.0);
    encoder_.encode(3, 108, 2.0);
    encoder_.encode(3, 109, 3.0);
    encoder_.encode(3, 110, 4.0);
    ChunkFinalizer::finalize(storage_, 3, storage_.open_chunks[3]);
    encoder_.encode(3, 111, 5.0);
    encoder_.encode(3, 112, 6.0);
    encoder_.encode(3, 113, 7.0);

    encoder_.encode(4, 111, 1.1);
    encoder_.encode(10, 111, 1.1);
    encoder_.encode(4, 112, 2.1);
    encoder_.encode(10, 112, 2.1);
    encoder_.encode(4, 113, 3.1);
    encoder_.encode(4, 114, 4.1);
    encoder_.encode(10, 113, STALE_NAN);
    ChunkFinalizer::finalize(storage_, 4, storage_.open_chunks[4]);
    encoder_.encode(4, 115, 5.1);
    encoder_.encode(4, 116, 6.1);

    encoder_.encode(5, 115, 1.1);
    encoder_.encode(5, 116, 2.1);
    encoder_.encode(5, 117, 3.1);
    encoder_.encode(5, 118, STALE_NAN);

    encoder_.encode(6, 119, 2.0);
    encoder_.encode(6, 120, 2.0);
    ChunkFinalizer::finalize(storage_, 6, storage_.open_chunks[6]);
    encoder_.encode(6, 121, 2.0);
    encoder_.encode(6, 122, 2.0);

    encoder_.encode(7, 121, -1.0);
    encoder_.encode(7, 122, -1.0);
    encoder_.encode(7, 123, -1.0);
  }
};

TEST_F(InstantQuerierFixture, InstantQueryEmptyChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, 1, TimeInterval{.min = 1, .max = 1});

  // Assert
  EXPECT_EQ((Sample{.timestamp = -1, .value = STALE_NAN}), sample);
}

TEST_F(InstantQuerierFixture, InstantQueryOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, 0, TimeInterval{.min = 1, .max = 1});

  // Assert
  EXPECT_EQ((Sample{.timestamp = 1, .value = 1.0}), sample);
}

TEST_F(InstantQuerierFixture, InstantQueryFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);
  encoder_.encode(0, 2, 1.0);

  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, 0, TimeInterval{.min = 1, .max = 1});

  // Assert
  EXPECT_EQ((Sample{1, 1.0}), sample);
}

TEST_F(InstantQuerierFixture, InstantQueryOutsideInterval) {
  // Arrange
  encoder_.encode(0, 1, 1.0);

  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, 0, TimeInterval{.min = 2, .max = 3});

  // Assert
  EXPECT_EQ((Sample{.timestamp = -1, .value = STALE_NAN}), sample);
}

TEST_P(InstantQuerierFixture, InstantQueryFilledChunks) {
  // Arrange
  fill_all();

  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, GetParam().request.ls_id, GetParam().request.time_interval);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, sample);
}

INSTANTIATE_TEST_SUITE_P(
    PickAfterOpenChunk,
    InstantQuerierFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 102}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 101, .value
      = STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 104}, .ls_id = 1}, .expected_sample = Sample{.timestamp = 103, .value
      = 1.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max =107}, .ls_id = 2}, .expected_sample = Sample{.timestamp = 106, .value =
      1.3}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 114}, .ls_id = 3}, .expected_sample = Sample{.timestamp = 113, .value
      = 7.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 117}, .ls_id = 4}, .expected_sample = Sample{.timestamp = 116, .value
      =
      6.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 119}, .ls_id = 5}, .expected_sample = Sample{.timestamp = 118, .value
      = STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 123}, .ls_id = 6}, .expected_sample = Sample{.timestamp = 122, .value
      = 2.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 124}, .ls_id = 7}, .expected_sample = Sample{.timestamp = 123, .value
      = -1.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 114}, .ls_id = 10}, .expected_sample = Sample{.timestamp = 113, .value
      = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(PickLastTimestampInFinalizedChunk,
                         InstantQuerierFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 110}, .ls_id = 3},
                           .expected_sample = Sample{.timestamp = 110, .value = 4.0}},
                           InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 114}, .ls_id = 4},
                           .expected_sample = Sample{.timestamp = 114, .value = 4.1}},
                           InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 120}, .ls_id = 6},
                           .expected_sample = Sample{.timestamp = 120, .value = 2.0}}));

INSTANTIATE_TEST_SUITE_P(
    PickInOpenChunk,
    InstantQuerierFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 100}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 100, .value
      = 1.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 102}, .ls_id = 1}, .expected_sample = Sample{.timestamp = 102, .value
      = 1.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 105}, .ls_id = 2}, .expected_sample = Sample{.timestamp = 105, .value
      = 1.2}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 108}, .ls_id = 3}, .expected_sample = Sample{.timestamp = 108, .value
      = 2.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 112}, .ls_id = 4}, .expected_sample = Sample{.timestamp = 112, .value
      = 2.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 118}, .ls_id = 5}, .expected_sample = Sample{.timestamp = 118, .value
      = STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 119}, .ls_id = 6}, .expected_sample = Sample{.timestamp = 119, .value
      = 2.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 121}, .ls_id = 7}, .expected_sample = Sample{.timestamp = 121, .value
      = -1.0}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 113}, .ls_id = 10}, .expected_sample = Sample{.timestamp = 113, .value
      = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(PickInFinalizedChunk,
                         InstantQuerierFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 112}, .ls_id = 3},
                           .expected_sample = Sample{.timestamp = 112, .value = 6.0}},
                           InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 115}, .ls_id = 4},
                           .expected_sample = Sample{.timestamp = 115, .value = 5.1}},
                           InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 122}, .ls_id = 6},
                           .expected_sample = Sample{.timestamp = 122, .value = 2.0}}));

INSTANTIATE_TEST_SUITE_P(
    PickBeforeAnyChunk,
    InstantQuerierFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 10}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 11}, .ls_id = 1}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 12}, .ls_id = 2}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 13}, .ls_id = 3}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 14}, .ls_id = 4}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 15}, .ls_id = 5}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 16}, .ls_id = 6}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 17}, .ls_id = 7}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 18}, .ls_id = 10}, .expected_sample = Sample{.timestamp = -1, .value =
      STALE_NAN}}));

class InstantQuerierOneChunkFixture : public InstantQuerierFixture {
protected:
  void fill_one_chunk() {
    encoder_.encode(0, 100, 1.1);
    encoder_.encode(0, 101, 2.1);
    encoder_.encode(0, 102, 3.1);
    encoder_.encode(0, 110, 4.1);
    encoder_.encode(0, 111, 5.1);
    ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

    encoder_.encode(0, 200, 6.1);
    encoder_.encode(0, 210, 7.1);
    encoder_.encode(0, 211, 8.1);
    encoder_.encode(0, 212, 9.1);
    encoder_.encode(0, 213, 10.1);
    ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

    encoder_.encode(0, 300, 11.1);
    encoder_.encode(0, 301, 12.1);
    encoder_.encode(0, 302, 13.1);
    encoder_.encode(0, 310, 14.1);
    encoder_.encode(0, 320, 15.1);
    ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

    encoder_.encode(0, 400, 16.1);
    encoder_.encode(0, 410, 17.1);
    encoder_.encode(0, 420, 18.1);
    encoder_.encode(0, 430, 19.1);
    encoder_.encode(0, 440, 20.1);
  }
};

TEST_P(InstantQuerierOneChunkFixture, InstantQueryFilledOneChunk) {
  // Arrange
  fill_one_chunk();

  Sample sample{.timestamp = -1, .value = STALE_NAN};

  // Act
  series_data::InstantQuerier::query_sample(sample, storage_, GetParam().request.ls_id, GetParam().request.time_interval);

  // Assert
  EXPECT_EQ(GetParam().expected_sample, sample);
}

INSTANTIATE_TEST_SUITE_P(PickAfterOpenChunk,
                         InstantQuerierOneChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 500}, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = 440, .value = 20.1}}));

INSTANTIATE_TEST_SUITE_P(PickBeforeAnyChunk,
                         InstantQuerierOneChunkFixture,
                         testing::Values(InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 13}, .ls_id = 0},
                           .expected_sample = Sample{.timestamp = -1, .value = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(
    PickInOpenChunk,
    InstantQuerierOneChunkFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 403}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 400, .value
      = 16.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 413}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 410, .value
      = 17.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 420}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 420, .value
      = 18.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 425}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 420, .value
      = 18.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 439}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 430, .value
      = 19.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 445, .max = 450}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      = STALE_NAN}}));

INSTANTIATE_TEST_SUITE_P(
    PickInFinalizedChunk,
    InstantQuerierOneChunkFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 100}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 100, .value
      = 1.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 107}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 102, .value
      = 3.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 208}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 200, .value
      = 6.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 212}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 212, .value
      = 9.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 300}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 300, .value
      = 11.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 315}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 310, .value
      = 14.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 104, .max = 100}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      = STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 205, .max = 208}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      = STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 305, .max = 309}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      = STALE_NAN}}
    ));

INSTANTIATE_TEST_SUITE_P(
    PickBetweenChunks,
    InstantQuerierOneChunkFixture,
    testing::Values(
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 153}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 111, .value
      =
      5.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 150, .max = 190}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 199}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 111, .value
      =
      5.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 250, .max = 299}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 214}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 213, .value
      =
      10.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 273}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 213, .value
      =
      10.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 333, .max = 388}, .ls_id = 0}, .expected_sample = Sample{.timestamp = -1, .value
      =
      STALE_NAN}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 333}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 320, .value
      =
      15.1}},
      InstantQuerierCase{.request = InstantQuerierRequest{.time_interval{.min = 0, .max = 368}, .ls_id = 0}, .expected_sample = Sample{.timestamp = 320, .value
      =
      15.1}}));
} // namespace