#include <gtest/gtest.h>

#include <iterator>

#include "entrypoint_types/serialized_data.h"
#include "series_data/chunk_finalizer.h"
#include "series_data/decoder/traits.h"
#include "series_data/encoder.h"
#include "series_data/encoder/sample.h"
#include "series_data/querier/querier.h"

namespace {

using series_data::ChunkFinalizer;
using series_data::DataStorage;
using series_data::Encoder;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::querier::Querier;
using series_data::serialization::SerializedDataView;
using Query = series_data::querier::Query<BareBones::Vector<PromPP::Primitives::LabelSetID>>;

class SerializedDataGoFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  Querier querier_{storage_};

  [[nodiscard]] static SampleList decode_chunk(const entrypoint_types::SerializedDataGo& data, uint32_t chunk_id) {
    SampleList decoded;
    std::ranges::copy(data.iterator(chunk_id), DecodeIteratorSentinel{}, std::back_inserter(decoded));
    return decoded;
  }
};

TEST_F(SerializedDataGoFixture, EmptyQueriedChunkListProducesNoChunks) {
  // Arrange

  // Act
  entrypoint_types::SerializedDataGo data{storage_, {}};

  // Assert
  EXPECT_EQ(0U, data.get_chunks_view().size());
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, data.next().first);
}

TEST_F(SerializedDataGoFixture, RoundTripsQueriedOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  const auto& queried_chunks = querier_.query(Query{.time_interval{.min = 1, .max = 5}, .label_set_ids = {0}});

  // Act
  entrypoint_types::SerializedDataGo data{storage_, queried_chunks};
  const auto next_series = data.next();
  const auto decoded = decode_chunk(data, 0);

  // Assert
  ASSERT_EQ(1U, data.get_chunks_view().size());
  EXPECT_EQ(0U, next_series.first);
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decoded);
}

TEST_F(SerializedDataGoFixture, RoundTripsQueriedFinalizedChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);
  ChunkFinalizer::finalize(storage_, 0, storage_.open_chunks[0]);

  encoder_.encode(0, 10, 10.0);

  const auto& queried_chunks = querier_.query(Query{.time_interval{.min = 1, .max = 5}, .label_set_ids = {0}});

  // Act
  entrypoint_types::SerializedDataGo data{storage_, queried_chunks};
  const auto next_series = data.next();
  const auto decoded = decode_chunk(data, 0);

  // Assert
  ASSERT_EQ(1U, data.get_chunks_view().size());
  EXPECT_EQ(0U, next_series.first);
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decoded);
}

TEST_F(SerializedDataGoFixture, NextReturnsChunkIdsForAllQueriedSeries) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  encoder_.encode(1, 1, 11.0);
  encoder_.encode(1, 2, 12.0);
  encoder_.encode(1, 3, 13.0);

  const auto& queried_chunks = querier_.query(Query{.time_interval{.min = 1, .max = 3}, .label_set_ids = {0, 1}});

  // Act
  entrypoint_types::SerializedDataGo data{storage_, queried_chunks};
  const auto series0 = data.next();
  const auto series1 = data.next();
  const auto end = data.next();

  // Assert
  ASSERT_EQ(2U, data.get_chunks_view().size());
  EXPECT_EQ(0U, series0.first);
  EXPECT_EQ(1U, series1.first);
  EXPECT_EQ(SerializedDataView::kNoMoreSeries, end.first);
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), decode_chunk(data, series0.second));
  EXPECT_EQ((SampleList{{1, 11.0}, {2, 12.0}, {3, 13.0}}), decode_chunk(data, series1.second));
}

}  // namespace
