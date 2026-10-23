#include <gtest/gtest.h>

#include <algorithm>
#include <iterator>
#include <span>
#include <vector>

#include "bare_bones/streams.h"
#include "entrypoint/types/querier.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Go::Slice;
using DataStorage = series_data::DataStorage<>;
using entrypoint::types::DataStorageWithArenas;
using series_data::Decoder;
using series_data::Encoder;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::unloading::Loader;
using series_data::unloading::Unloader;
using InstantQuerierWrapper = entrypoint::types::InstantQuerierWithArgumentsWrapper<std::vector<LabelSetID>, std::span<Sample>, DataStorageWithArenas>;
using RangeQuery = series_data::querier::Query<Slice<LabelSetID>>;

class RangeQuerierWrapperFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream unloaded_chunks_;
  entrypoint::types::SerializedDataPtr serialized_data_;

  static RangeQuery query_for(LabelSetID label_set_id, int64_t min, int64_t max) {
    Slice<LabelSetID> label_set_ids;
    label_set_ids.push_back(label_set_id);
    return RangeQuery{.time_interval{.min = min, .max = max}, .label_set_ids = std::move(label_set_ids)};
  }

  [[nodiscard]] SampleList decode_chunk(uint32_t chunk_id) const {
    SampleList decoded;
    std::ranges::copy(std::visit([chunk_id](auto& serialized_data) { return serialized_data.samples_iterator(chunk_id); }, *serialized_data_),
                      DecodeIteratorSentinel{}, std::back_inserter(decoded));
    return decoded;
  }

  [[nodiscard]] size_t serialized_chunks_count() const {
    return std::visit([](auto& serialized_data) { return serialized_data.get_chunks_view().size(); }, *serialized_data_);
  }

  [[nodiscard]] entrypoint::types::SerializedDataPtr* serialized_data_ptr() noexcept { return &serialized_data_; }

  void unload_open_chunks() {
    Unloader unloader{storage_};
    unloader.create_snapshot(unloaded_chunks_);
    unloader.unload();
  }

  void load_unloaded_chunks(LabelSetID label_set_id) {
    const std::vector label_set_ids{label_set_id};
    Loader loader{storage_, label_set_ids, static_cast<uint32_t>(label_set_ids.size())};
    loader.load_next(unloaded_chunks_.span<const uint8_t>());
    loader.load_finalize();
  }
};

TEST_F(RangeQuerierWrapperFixture, QueryWritesSerializedDataToPreparedMemory) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  const auto query = query_for(0, 1, 1);
  const auto was_null_before_prepare = serialized_data_ == nullptr;
  entrypoint::types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, {}, serialized_data_ptr(), {}};

  // Act
  wrapper.query();

  // Assert
  EXPECT_TRUE(was_null_before_prepare);
  ASSERT_NE(nullptr, serialized_data_);
}

TEST_F(RangeQuerierWrapperFixture, QueryFinalizeWritesSerializedDataToPreparedMemory) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload_open_chunks();

  const auto query = query_for(0, 1, 3);
  const auto was_null_before_prepare = serialized_data_ == nullptr;
  entrypoint::types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, {}, serialized_data_ptr(), {}};

  // Act
  wrapper.query();
  const auto need_loading = wrapper.need_loading();
  load_unloaded_chunks(0);
  wrapper.query_finalize();

  // Assert
  ASSERT_TRUE(need_loading);
  EXPECT_TRUE(was_null_before_prepare);
  ASSERT_NE(nullptr, serialized_data_);
}

class InstantQuerierWrapperFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream unloaded_chunks_;
  std::vector<LabelSetID> label_set_ids_{0};
  std::vector<Sample> samples_{Sample{.timestamp = -1, .value = STALE_NAN}};

  void encode_open_chunk() {
    encoder_.encode(0, 1, 1.0);
    encoder_.encode(0, 2, 2.0);
    encoder_.encode(0, 3, 3.0);
    encoder_.encode(0, 4, 4.0);
    encoder_.encode(0, 5, 5.0);
  }

  void unload_open_chunks() {
    Unloader unloader{storage_};
    unloader.create_snapshot(unloaded_chunks_);
    unloader.unload();
  }

  void load_unloaded_chunks() {
    Loader loader{storage_, label_set_ids_, static_cast<uint32_t>(label_set_ids_.size())};
    loader.load_next(unloaded_chunks_.span<const uint8_t>());
    loader.load_finalize();
  }
};

TEST_F(InstantQuerierWrapperFixture, QueryReturnsSampleAtTimestamp) {
  // Arrange
  encode_open_chunk();
  std::span<Sample> samples_view{samples_};
  InstantQuerierWrapper wrapper{storage_, label_set_ids_, 3, samples_view};

  // Act
  wrapper.query();

  // Assert
  EXPECT_EQ((Sample{.timestamp = 3, .value = 3.0}), samples_[0]);
  EXPECT_FALSE(wrapper.need_loading());
}

TEST_F(InstantQuerierWrapperFixture, QueryKeepsDefaultSampleWhenSeriesHasNoPointBeforeTimestamp) {
  // Arrange
  encoder_.encode(0, 10, 10.0);
  std::span<Sample> samples_view{samples_};
  InstantQuerierWrapper wrapper{storage_, label_set_ids_, 5, samples_view};

  // Act
  wrapper.query();

  // Assert
  EXPECT_EQ((Sample{.timestamp = -1, .value = STALE_NAN}), samples_[0]);
  EXPECT_FALSE(wrapper.need_loading());
}

TEST_F(InstantQuerierWrapperFixture, QueryRequestsLoadingForUnloadedSeriesThenFinalizeReturnsSample) {
  // Arrange
  encode_open_chunk();
  unload_open_chunks();

  std::span<Sample> samples_view{samples_};
  InstantQuerierWrapper wrapper{storage_, label_set_ids_, 3, samples_view};

  // Act
  wrapper.query();
  const auto need_loading = wrapper.need_loading();
  const auto series_to_load_0 = wrapper.series_to_load().is_set(0);
  load_unloaded_chunks();
  wrapper.query_finalize();

  // Assert
  ASSERT_TRUE(need_loading);
  EXPECT_TRUE(series_to_load_0);
  EXPECT_EQ((Sample{.timestamp = 3, .value = 3.0}), samples_[0]);
}

TEST_F(RangeQuerierWrapperFixture, QuerySerializesMatchingOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  const auto query = query_for(0, 2, 4);
  entrypoint::types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, {}, serialized_data_ptr(), {}};

  // Act
  wrapper.query();
  const auto decoded = decode_chunk(0);

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_);
  ASSERT_EQ(1U, serialized_chunks_count());
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decoded);
}

TEST_F(RangeQuerierWrapperFixture, QuerySerializesEmptyResultWhenSeriesDoesNotMatchInterval) {
  // Arrange
  encoder_.encode(0, 10, 10.0);
  const auto query = query_for(0, 1, 5);
  entrypoint::types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, {}, serialized_data_ptr(), {}};

  // Act
  wrapper.query();

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_);
  EXPECT_EQ(0U, serialized_chunks_count());
}

TEST_F(RangeQuerierWrapperFixture, QueryDefersSerializationUntilUnloadedSeriesIsLoaded) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload_open_chunks();

  const auto query = query_for(0, 1, 3);
  entrypoint::types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, {}, serialized_data_ptr(), {}};

  // Act
  wrapper.query();

  const auto need_loading = wrapper.need_loading();
  const auto series_to_load_0 = wrapper.series_to_load().is_set(0);
  const auto was_null_before_finalize = serialized_data_ == nullptr;

  load_unloaded_chunks(0);
  wrapper.query_finalize();

  // Assert
  ASSERT_TRUE(need_loading);
  EXPECT_TRUE(series_to_load_0);
  EXPECT_TRUE(was_null_before_finalize);
  ASSERT_NE(nullptr, serialized_data_);
  ASSERT_EQ(1U, serialized_chunks_count());
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), decode_chunk(0));
}

}  // namespace
