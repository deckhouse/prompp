#include <gtest/gtest.h>

#include <algorithm>
#include <cstddef>
#include <iterator>
#include <memory>
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
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;
using series_data::unloading::Loader;
using series_data::unloading::Unloader;
using InstantQuerierWrapper = entrypoint_types::InstantQuerierWithArgumentsWrapper<std::vector<LabelSetID>, std::span<Sample>>;
using RangeQuery = series_data::querier::Query<Slice<LabelSetID>>;

template <class T>
class UninitializedMemory {
 public:
  UninitializedMemory() { std::ranges::fill(storage_, kDefaultValue); }

  ~UninitializedMemory() {
    if (!has_default_value()) {
      std::destroy_at(ptr());
    }
  }

  [[nodiscard]] T* ptr() noexcept { return reinterpret_cast<T*>(storage_); }
  [[nodiscard]] const T* ptr() const noexcept { return reinterpret_cast<const T*>(storage_); }
  [[nodiscard]] T& value() noexcept { return *ptr(); }
  [[nodiscard]] const T& value() const noexcept { return *ptr(); }
  [[nodiscard]] bool has_default_value() const noexcept {
    return std::ranges::all_of(storage_, [](std::byte byte) { return byte == kDefaultValue; });
  }

 private:
  static constexpr auto kDefaultValue = std::byte{0x5a};

  alignas(T) std::byte storage_[sizeof(T)];
};

class RangeQuerierUninitializedMemoryFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream unloaded_chunks_;
  UninitializedMemory<entrypoint_types::SerializedDataPtr> serialized_data_memory_;

  RangeQuery query_for(LabelSetID label_set_id, int64_t min, int64_t max) {
    Slice<LabelSetID> label_set_ids;
    label_set_ids.push_back(label_set_id);
    return RangeQuery{.time_interval{.min = min, .max = max}, .label_set_ids = std::move(label_set_ids)};
  }

  [[nodiscard]] entrypoint_types::SerializedDataPtr* serialized_data_ptr() noexcept { return serialized_data_memory_.ptr(); }

  void unload_open_chunks() {
    Unloader unloader{storage_};
    unloader.create_snapshot(unloaded_chunks_);
    unloader.unload();
  }

  void load_unloaded_chunks(LabelSetID label_set_id) {
    std::vector label_set_ids{label_set_id};
    Loader loader{storage_, label_set_ids, static_cast<uint32_t>(label_set_ids.size())};
    loader.load_next(unloaded_chunks_.span<const uint8_t>());
    loader.load_finalize();
  }
};

TEST_F(RangeQuerierUninitializedMemoryFixture, QueryWritesSerializedDataToPreparedMemory) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  auto query = query_for(0, 1, 1);
  const auto was_default_before_prepare = serialized_data_memory_.has_default_value();
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_ptr()};

  // Act
  wrapper.query();

  // Assert
  EXPECT_TRUE(was_default_before_prepare);
  EXPECT_FALSE(serialized_data_memory_.has_default_value());
  ASSERT_NE(nullptr, serialized_data_memory_.value().get());
}

TEST_F(RangeQuerierUninitializedMemoryFixture, QueryFinalizeWritesSerializedDataToPreparedMemory) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload_open_chunks();

  auto query = query_for(0, 1, 3);
  const auto was_default_before_prepare = serialized_data_memory_.has_default_value();
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_ptr()};

  // Act
  wrapper.query();
  const auto need_loading = wrapper.need_loading();
  load_unloaded_chunks(0);
  wrapper.query_finalize();

  // Assert
  ASSERT_TRUE(need_loading);
  EXPECT_TRUE(was_default_before_prepare);
  EXPECT_FALSE(serialized_data_memory_.has_default_value());
  ASSERT_NE(nullptr, serialized_data_memory_.value().get());
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

class RangeQuerierWrapperFixture : public testing::Test {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};
  BareBones::ShrinkedToFitOStringStream unloaded_chunks_;
  UninitializedMemory<entrypoint_types::SerializedDataPtr> serialized_data_memory_;
  entrypoint_types::SerializedDataPtr serialized_data_;

  RangeQuery query_for(LabelSetID label_set_id, int64_t min, int64_t max) {
    Slice<LabelSetID> label_set_ids;
    label_set_ids.push_back(label_set_id);
    return RangeQuery{.time_interval{.min = min, .max = max}, .label_set_ids = std::move(label_set_ids)};
  }

  [[nodiscard]] SampleList decode_chunk(uint32_t chunk_id) const {
    SampleList decoded;
    std::ranges::copy(serialized_data_->iterator(chunk_id), DecodeIteratorSentinel{}, std::back_inserter(decoded));
    return decoded;
  }

  [[nodiscard]] entrypoint_types::SerializedDataPtr* serialized_data_ptr() noexcept { return serialized_data_memory_.ptr(); }

  void take_serialized_data() { serialized_data_ = std::move(serialized_data_memory_.value()); }

  void unload_open_chunks() {
    Unloader unloader{storage_};
    unloader.create_snapshot(unloaded_chunks_);
    unloader.unload();
  }

  void load_unloaded_chunks(LabelSetID label_set_id) {
    std::vector label_set_ids{label_set_id};
    Loader loader{storage_, label_set_ids, static_cast<uint32_t>(label_set_ids.size())};
    loader.load_next(unloaded_chunks_.span<const uint8_t>());
    loader.load_finalize();
  }
};

TEST_F(RangeQuerierWrapperFixture, QuerySerializesMatchingOpenChunk) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);
  encoder_.encode(0, 4, 4.0);
  encoder_.encode(0, 5, 5.0);

  auto query = query_for(0, 2, 4);
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_ptr()};

  // Act
  wrapper.query();
  take_serialized_data();
  const auto decoded = decode_chunk(0);

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_);
  ASSERT_EQ(1U, serialized_data_->get_chunks_view().size());
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decoded);
}

TEST_F(RangeQuerierWrapperFixture, QuerySerializesEmptyResultWhenSeriesDoesNotMatchInterval) {
  // Arrange
  encoder_.encode(0, 10, 10.0);
  auto query = query_for(0, 1, 5);
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_ptr()};

  // Act
  wrapper.query();
  take_serialized_data();

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_);
  EXPECT_EQ(0U, serialized_data_->get_chunks_view().size());
}

TEST_F(RangeQuerierWrapperFixture, QueryDefersSerializationUntilUnloadedSeriesIsLoaded) {
  // Arrange
  encoder_.encode(0, 1, 1.0);
  encoder_.encode(0, 2, 2.0);
  encoder_.encode(0, 3, 3.0);

  unload_open_chunks();

  auto query = query_for(0, 1, 3);
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_ptr()};

  // Act
  wrapper.query();

  const auto need_loading = wrapper.need_loading();
  const auto series_to_load_0 = wrapper.series_to_load().is_set(0);
  const auto was_default_before_finalize = serialized_data_memory_.has_default_value();

  load_unloaded_chunks(0);
  wrapper.query_finalize();
  take_serialized_data();

  // Assert
  ASSERT_TRUE(need_loading);
  EXPECT_TRUE(series_to_load_0);
  EXPECT_TRUE(was_default_before_finalize);
  ASSERT_NE(nullptr, serialized_data_);
  ASSERT_EQ(1U, serialized_data_->get_chunks_view().size());
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}}), decode_chunk(0));
}

}  // namespace
