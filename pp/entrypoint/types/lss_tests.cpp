#include <gtest/gtest.h>

#include <array>
#include <barrier>
#include <cstdint>
#include <memory>
#include <thread>
#include <tuple>
#include <utility>
#include <variant>

#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "entrypoint/types/lss.h"
#include "primitives/label_set.h"
#include "series_index/queryable_encoding_bimap.h"

#if PROMPP(asan)
#  include <sanitizer/asan_interface.h>
#endif

namespace {

using entrypoint::types::create_lss;
using entrypoint::types::create_snapshot_lss;
using entrypoint::types::EncodingBimap;
using entrypoint::types::LssType;
using entrypoint::types::QueryableEncodingBimap;
using entrypoint::types::ReallocationsDetector;
using entrypoint::types::ShrinkAwareSnapshotLSS;
using entrypoint::types::SnapshotLSS;
using PromPP::Primitives::LabelViewSet;

class LssFixture : public testing::Test {};

TEST_F(LssFixture, CreateLssEncodingBimapSelectsExpectedAlternative) {
  // Arrange

  // Act
  const auto lss = create_lss(LssType::kEncodingBimap);

  // Assert
  EXPECT_TRUE(std::holds_alternative<EncodingBimap>(*lss));
}

TEST_F(LssFixture, CreateLssQueryableEncodingBimapSelectsExpectedAlternative) {
  // Arrange

  // Act
  const auto lss = create_lss(LssType::kQueryableEncodingBimap);

  // Assert
  EXPECT_TRUE(std::holds_alternative<QueryableEncodingBimap>(*lss));
}

TEST_F(LssFixture, CreateLssRejectsUnknownType) {
  // Arrange
  constexpr auto unknown_type = static_cast<LssType>(-1);

  // Act

  // Assert
  EXPECT_THROW((void)create_lss(unknown_type), BareBones::Exception);
}

TEST_F(LssFixture, CreateSnapshotFromEncodingBimapProducesPlainSnapshot) {
  // Arrange
  const auto lss = create_lss(LssType::kEncodingBimap);
  std::get<EncodingBimap>(*lss).find_or_emplace(LabelViewSet{{"job", "a"}});

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  EXPECT_TRUE(std::holds_alternative<SnapshotLSS>(*snapshot));
}

template <class DecodingTable, class SortingIndex, class SeriesIds, class LsIdVector>
using QueryableEncodingBimapCopier = series_index::QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

class SnapshotLssFixture : public testing::Test {
 protected:
  static constexpr uint32_t kShrinkBoundary = 3U;

  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};
  const LabelViewSet ls3_{{"job", "d"}};
  const LabelViewSet ls4_{{"job", "e"}};

  entrypoint::types::LssVariantPtr create_queryable_lss() const {
    auto lss = create_lss(LssType::kQueryableEncodingBimap);
    auto& bimap = std::get<QueryableEncodingBimap>(*lss);

    bimap.find_or_emplace(ls0_);
    bimap.find_or_emplace(ls1_);
    bimap.find_or_emplace(ls2_);
    bimap.find_or_emplace(ls3_);
    bimap.find_or_emplace(ls4_);

    bimap.build_deferred_indexes();

    return lss;
  }

  entrypoint::types::LssVariantPtr create_fixed_lss() const {
    auto lss = create_queryable_lss();
    std::get<QueryableEncodingBimap>(*lss).set_pending_shrink_boundary(kShrinkBoundary);

    return lss;
  }

  entrypoint::types::LssVariantPtr create_shrunk_lss() const {
    QueryableEncodingBimap seeded_lss;
    seeded_lss.find_or_emplace(ls0_);
    seeded_lss.find_or_emplace(ls1_);
    seeded_lss.find_or_emplace(ls2_);
    seeded_lss.find_or_emplace(ls3_);
    seeded_lss.find_or_emplace(ls4_);

    seeded_lss.build_deferred_indexes();

    auto lss = create_lss(LssType::kQueryableEncodingBimap);
    auto& bimap = std::get<QueryableEncodingBimap>(*lss);
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    QueryableEncodingBimapCopier copier(seeded_lss, seeded_lss.sorting_index(), seeded_lss.added_series(), bimap, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    std::ignore = bimap.find_or_emplace(ls1_);
    std::ignore = bimap.find_or_emplace(ls3_);
    std::ignore = bimap.find_or_emplace(ls4_);
    bimap.build_deferred_indexes();

    dst_src_ids_mapping.clear();
    QueryableEncodingBimap lss_copy;
    QueryableEncodingBimapCopier shrink_copier(bimap, bimap.sorting_index(), bimap.added_series(), lss_copy, dst_src_ids_mapping);
    shrink_copier.copy_added_series_and_build_indexes();
    bimap.set_pending_shrink_boundary(kShrinkBoundary);
    const SnapshotLSS resolve_snapshot(lss_copy);
    bimap.finalize_copy_and_shrink(resolve_snapshot, dst_src_ids_mapping);
    return lss;
  }
};

TEST_F(SnapshotLssFixture, ResolvesNormalQueryableLss) {
  // Arrange
  const auto lss = create_queryable_lss();

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  ASSERT_TRUE(std::holds_alternative<SnapshotLSS>(*snapshot));
  EXPECT_EQ(ls0_, std::get<SnapshotLSS>(*snapshot)[0]);
  EXPECT_EQ(ls4_, std::get<SnapshotLSS>(*snapshot)[4]);
}

TEST_F(SnapshotLssFixture, ConcurrentOwnerAndSnapshotRelease) {
  // Arrange
  auto lss = create_queryable_lss();
  auto snapshot = create_snapshot_lss(*lss);
  auto surviving_snapshot = create_snapshot_lss(*lss);
  // Keep a handle to the real sorting buffer to check both releases decremented its count.
  auto sorting_buffer = std::get<QueryableEncodingBimap>(*lss).sorting_index().index;
  ASSERT_EQ(4U, sorting_buffer.shared_ptr().use_count());
  std::array<uint32_t, 2> series_ids{4U, 0U};
  std::barrier release(2);

  // Act
  std::jthread snapshot_cleanup([snapshot = std::move(snapshot), &release]() mutable {
    release.arrive_and_wait();
    snapshot.reset();
  });
  release.arrive_and_wait();
  lss.reset();
  snapshot_cleanup.join();

  const auto remaining_references = sorting_buffer.shared_ptr().use_count();
  sorting_buffer = {};
  const auto& remaining = std::get<SnapshotLSS>(*surviving_snapshot);
  remaining.sorting_index().sort(series_ids);

  // Assert
  EXPECT_EQ(2U, remaining_references);
  EXPECT_EQ(ls0_, remaining[0]);
  EXPECT_EQ(ls4_, remaining[4]);
  EXPECT_EQ((std::array<uint32_t, 2>{0U, 4U}), series_ids);
#if PROMPP(asan)
  const auto* buffer_address = remaining.sorting_index().index.data();
  surviving_snapshot.reset();
  EXPECT_TRUE(__asan_address_is_poisoned(buffer_address));
#endif
}

TEST_F(SnapshotLssFixture, FromFixedQueryableLssIsShrinkAware) {
  // Arrange
  const auto lss = create_fixed_lss();

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  EXPECT_TRUE(std::holds_alternative<entrypoint::types::ShrinkAwareSnapshotLSS>(*snapshot));
}

TEST_F(SnapshotLssFixture, ShrinkAwareResolvesSurvivingPreBoundarySeries) {
  // Arrange
  const auto lss = create_shrunk_lss();

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  ASSERT_TRUE(std::holds_alternative<ShrinkAwareSnapshotLSS>(*snapshot));
  EXPECT_EQ(ls1_, std::get<ShrinkAwareSnapshotLSS>(*snapshot)[1]);
}

TEST_F(SnapshotLssFixture, ShrinkAwareHidesDroppedPreBoundarySeries) {
  // Arrange
  const auto lss = create_shrunk_lss();

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  ASSERT_TRUE(std::holds_alternative<ShrinkAwareSnapshotLSS>(*snapshot));
  EXPECT_EQ(0U, std::get<ShrinkAwareSnapshotLSS>(*snapshot)[0].size());
  EXPECT_EQ(0U, std::get<ShrinkAwareSnapshotLSS>(*snapshot)[2].size());
}

TEST_F(SnapshotLssFixture, ShrinkAwareResolvesPostBoundarySeries) {
  // Arrange
  const auto lss = create_shrunk_lss();

  // Act
  const auto snapshot = create_snapshot_lss(*lss);

  // Assert
  ASSERT_TRUE(std::holds_alternative<ShrinkAwareSnapshotLSS>(*snapshot));
  EXPECT_EQ(ls3_, std::get<ShrinkAwareSnapshotLSS>(*snapshot)[3]);
  EXPECT_EQ(ls4_, std::get<ShrinkAwareSnapshotLSS>(*snapshot)[4]);
}

class ReallocationsDetectorFixture : public ::testing::Test {
 protected:
  QueryableEncodingBimap lss_;
};

TEST_F(ReallocationsDetectorFixture, ReportsReallocOnEmplace) {
  // Arrange
  const ReallocationsDetector detector(lss_);

  // Act
  lss_.find_or_emplace(LabelViewSet{{"job", "a"}});

  // Assert
  EXPECT_TRUE(detector.has_reallocations());
}

TEST_F(ReallocationsDetectorFixture, StaysQuietWithoutChanges) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "a"}});
  lss_.build_deferred_indexes();

  // Act
  const ReallocationsDetector detector(lss_);

  // Assert
  EXPECT_FALSE(detector.has_reallocations());
}

}  // namespace
