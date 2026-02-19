#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder.h"
#include "series_index/queried_series.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "status.h"

namespace {

using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, series_index::trie::CedarTrie>;
using head::StatusGetterLSS;
using PromPP::Primitives::LabelViewSet;
using series_data::DataStorage;
using series_data::Encoder;
using QuerySource = series_index::QueriedSeries::Source;

using Status = head::Status<std::string_view, std::vector>;

class StatusFixture : public ::testing::Test {
 protected:
  static constexpr size_t kTopItemsCount = 10;

  QueryableEncodingBimap lss_;
  DataStorage storage_;

  [[nodiscard]] Status get_status() const {
    Status status;
    StatusGetterLSS<QueryableEncodingBimap, Status>{lss_, kTopItemsCount}.get(status);
    status.min_max_timestamp = series_data::Decoder::get_time_interval(storage_);
    status.chunk_count = storage_.chunks().non_empty_chunk_count();
    return status;
  }
};

TEST_F(StatusFixture, EmptyLssAndStorage) {
  // Arrange

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ(Status{}, status);
}

TEST_F(StatusFixture, FinalizedChunk) {
  // Arrange
  Encoder<2> encoder{storage_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(0, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 2}), status);
}

TEST_F(StatusFixture, FinalizedTimestreamChunk) {
  // Arrange
  Encoder<2> encoder{storage_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(1, 1, 1.0);
  encoder.encode(0, 2, 1.0);
  encoder.encode(1, 2, 1.0);
  encoder.encode(0, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 3}), status);
}

TEST_F(StatusFixture, OpenedChunk) {
  // Arrange
  Encoder<2> encoder{storage_};
  encoder.encode(0, 1, 1.0);
  encoder.encode(1, 2, 1.0);
  encoder.encode(2, 3, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 3}, .chunk_count = 3}), status);
}

TEST_F(StatusFixture, EmptyChunk) {
  // Arrange
  Encoder<2> encoder{storage_};
  encoder.encode(5, 1, 1.0);

  // Act
  const auto status = get_status();

  // Assert
  EXPECT_EQ((Status{.min_max_timestamp = {.min = 1, .max = 1}, .chunk_count = 1}), status);
}

}  // namespace
