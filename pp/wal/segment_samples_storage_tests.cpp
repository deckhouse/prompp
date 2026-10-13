#include <gtest/gtest.h>

#include "primitives/primitives.h"
#include "primitives/sample.h"
#include "wal/segment_samples_storage.h"

namespace {

using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::WAL::SegmentSamplesStorage;

struct SeriesSample {
  Sample sample;
  LabelSetID ls_id;

  bool operator==(const SeriesSample&) const noexcept = default;
};

struct SegmentSamplesAddCase {
  std::vector<SeriesSample> samples;
  std::vector<SeriesSample> expected_samples;
  uint32_t samples_count;
  uint32_t series_count;
  Timestamp earliest_sample;
  Timestamp latest_sample;
};

class SampleStorageFixture : public testing::TestWithParam<SegmentSamplesAddCase> {
 protected:
  SegmentSamplesStorage samples_;

  void add() {
    for (auto& series_sample : GetParam().samples) {
      samples_.add(series_sample.ls_id, series_sample.sample);
    }
  }

  [[nodiscard]] std::vector<SeriesSample> get() const noexcept {
    std::vector<SeriesSample> samples;
    samples_.for_each([&samples](LabelSetID ls_id, Timestamp timestamp, Sample::value_type value) {
      samples.emplace_back(SeriesSample{.sample = {timestamp, value}, .ls_id = ls_id});
    });
    return samples;
  }
};

TEST_P(SampleStorageFixture, TestAdd) {
  // Arrange

  // Act
  add();

  // Assert
  EXPECT_EQ(GetParam().samples_count, samples_.samples_count());
  EXPECT_EQ(GetParam().series_count, samples_.series_count());
  EXPECT_EQ(GetParam().earliest_sample, samples_.earliest_sample());
  EXPECT_EQ(GetParam().latest_sample, samples_.latest_sample());
  EXPECT_EQ(GetParam().expected_samples, get());
}

TEST_F(SampleStorageFixture, TestFillFirstSampleAddedAtTsNs) {
  // Arrange
  const auto start = samples_.first_sample_added_at_ts_ns();

  // Act
  samples_.add(0, Sample{101, 1.0});
  const auto filled = samples_.first_sample_added_at_ts_ns();
  samples_.add(0, Sample{102, 1.0});

  // Assert
  EXPECT_EQ(0, start);
  EXPECT_EQ(filled, samples_.first_sample_added_at_ts_ns());
  EXPECT_NE(0, filled);
}

TEST_F(SampleStorageFixture, Clear) {
  // Arrange
  samples_.add(0, Sample{101, 1.0});

  // Act
  samples_.clear();

  // Assert
  EXPECT_EQ(0U, samples_.samples_count());
  EXPECT_EQ(0U, samples_.series_count());
  EXPECT_EQ(std::numeric_limits<Timestamp>::max(), samples_.earliest_sample());
  EXPECT_EQ(0, samples_.latest_sample());
  EXPECT_EQ(0, samples_.first_sample_added_at_ts_ns());
  EXPECT_EQ(std::vector<SeriesSample>{}, get());
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         SampleStorageFixture,
                         testing::Values(SegmentSamplesAddCase{
                             .samples = {{.sample = {101, 1.0}, .ls_id = 0}},
                             .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}},
                             .samples_count = 1,
                             .series_count = 1,
                             .earliest_sample = 101,
                             .latest_sample = 101,
                         }));

INSTANTIATE_TEST_SUITE_P(
    ManySamplesForOneSerie,
    SampleStorageFixture,
    testing::Values(
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {103, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {103, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 103,
        }));

INSTANTIATE_TEST_SUITE_P(
    SortSamplesByTimestamp,
    SampleStorageFixture,
    testing::Values(
        SegmentSamplesAddCase{
            .samples = {{.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {100, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {100, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 100,
            .latest_sample = 102,
        }));

INSTANTIATE_TEST_SUITE_P(
    TwoSeries,
    SampleStorageFixture,
    testing::Values(
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1}, {.sample = {101, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 1}},
            .samples_count = 2,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 101,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1000}, {.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 1000}},
            .samples_count = 3,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1000},
                        {.sample = {102, 1.0}, .ls_id = 1000},
                        {.sample = {101, 1.0}, .ls_id = 0},
                        {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0},
                                 {.sample = {102, 1.0}, .ls_id = 0},
                                 {.sample = {101, 1.0}, .ls_id = 1000},
                                 {.sample = {102, 1.0}, .ls_id = 1000}},
            .samples_count = 4,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 102,
        }));

INSTANTIATE_TEST_SUITE_P(
    DontSkipNonUniqueSamples,
    SampleStorageFixture,
    testing::Values(
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 101,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        SegmentSamplesAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {102, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {102, 2.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        }));

}  // namespace