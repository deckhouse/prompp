#include <gtest/gtest.h>

#include <cstddef>
#include <iterator>
#include <memory>
#include <span>
#include <vector>

#include "bare_bones/streams.h"
#include "entrypoint_types/querier.h"
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

class SerializedDataPtrStorage {
 public:
  ~SerializedDataPtrStorage() {
    if (constructed_) {
      std::destroy_at(ptr());
    }
  }

  [[nodiscard]] entrypoint_types::SerializedDataPtr* ptr() noexcept { return reinterpret_cast<entrypoint_types::SerializedDataPtr*>(storage_); }
  [[nodiscard]] const entrypoint_types::SerializedDataPtr* ptr() const noexcept {
    return reinterpret_cast<const entrypoint_types::SerializedDataPtr*>(storage_);
  }
  [[nodiscard]] const entrypoint_types::SerializedDataGo* get() const noexcept { return ptr()->get(); }

  void mark_constructed() noexcept { constructed_ = true; }

 private:
  alignas(entrypoint_types::SerializedDataPtr) std::byte storage_[sizeof(entrypoint_types::SerializedDataPtr)];
  bool constructed_{false};
};

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

TEST_F(InstantQuerierWrapperFixture, QueryReturnsSampleBeforeTimestamp) {
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
  SerializedDataPtrStorage serialized_data_;

  RangeQuery query_for(LabelSetID label_set_id, int64_t min, int64_t max) {
    Slice<LabelSetID> label_set_ids;
    label_set_ids.push_back(label_set_id);
    return RangeQuery{.time_interval{.min = min, .max = max}, .label_set_ids = std::move(label_set_ids)};
  }

  [[nodiscard]] SampleList decode_chunk(uint32_t chunk_id) const {
    SampleList decoded;
    std::ranges::copy((*serialized_data_.ptr())->iterator(chunk_id), DecodeIteratorSentinel{}, std::back_inserter(decoded));
    return decoded;
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
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_.ptr()};

  // Act
  wrapper.query();
  serialized_data_.mark_constructed();
  const auto decoded = decode_chunk(0);

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_.get());
  ASSERT_EQ(1U, serialized_data_.get()->get_chunks_view().size());
  EXPECT_EQ((SampleList{{1, 1.0}, {2, 2.0}, {3, 3.0}, {4, 4.0}, {5, 5.0}}), decoded);
}

TEST_F(RangeQuerierWrapperFixture, QuerySerializesEmptyResultWhenSeriesDoesNotMatchInterval) {
  // Arrange
  encoder_.encode(0, 10, 10.0);
  auto query = query_for(0, 1, 5);
  entrypoint_types::RangeQuerierWithArgumentsWrapperV2 wrapper{storage_, query, serialized_data_.ptr()};

  // Act
  wrapper.query();
  serialized_data_.mark_constructed();

  // Assert
  ASSERT_FALSE(wrapper.need_loading());
  ASSERT_NE(nullptr, serialized_data_.get());
  EXPECT_EQ(0U, serialized_data_.get()->get_chunks_view().size());
}

}  // namespace
