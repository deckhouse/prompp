#include <utility>
#include <vector>

#include <gmock/gmock.h>

#include "series_index/reverse_index.h"

namespace {

using series_index::LabelReverseIndex;
using series_index::SeriesIdSequence;
using series_index::SeriesIdSequenceSnapshot;
using series_index::SeriesReverseIndex;

class SeriesIdSequenceSnapshotFixture : public ::testing::Test {
 protected:
  SeriesIdSequence sequence_{};
};

TEST_F(SeriesIdSequenceSnapshotFixture, SnapshotOfEmptySequenceIsEmpty) {
  // Arrange

  // Act
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre());
  EXPECT_EQ(0U, snapshot.count());
}

TEST_F(SeriesIdSequenceSnapshotFixture, SnapshotCopiesValuesAndCount) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);

  // Act
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2));
  EXPECT_EQ(2U, snapshot.count());
}

TEST_F(SeriesIdSequenceSnapshotFixture, AppendDoesNotExtendSnapshot) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Act
  sequence_.push_back(3);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2));
  EXPECT_EQ(2U, snapshot.count());
}

TEST_F(SeriesIdSequenceSnapshotFixture, EmptySnapshotDoesNotFollowSourceAppend) {
  // Arrange
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Act
  sequence_.push_back(1);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre());
}

TEST_F(SeriesIdSequenceSnapshotFixture, ClearDoesNotInvalidateSnapshot) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  sequence_.push_back(4);
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Act
  sequence_.clear();

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2, 4));
  EXPECT_TRUE(sequence_.empty());
}

TEST_F(SeriesIdSequenceSnapshotFixture, ReuseAfterClearDoesNotChangeSnapshot) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  sequence_.push_back(4);
  const SeriesIdSequenceSnapshot snapshot(sequence_);
  sequence_.clear();

  // Act
  sequence_.push_back(0);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2, 4));
  EXPECT_THAT(sequence_, testing::ElementsAre(0));
}

TEST_F(SeriesIdSequenceSnapshotFixture, AssigningSourceDoesNotChangeSnapshotCopy) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  SeriesIdSequenceSnapshot snapshot(sequence_);
  const auto copy = snapshot;
  sequence_.push_back(3);

  // Act
  snapshot = sequence_;

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2, 3));
  EXPECT_THAT(copy, testing::ElementsAre(1, 2));
}

TEST_F(SeriesIdSequenceSnapshotFixture, SnapshotSurvivesSourceDestruction) {
  // Arrange
  SeriesIdSequenceSnapshot snapshot;

  // Act
  {
    SeriesIdSequence source;
    source.push_back(1);
    source.push_back(2);
    source.push_back(4);
    snapshot = source;
  }

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2, 4));
}

TEST_F(SeriesIdSequenceSnapshotFixture, SnapshotSurvivesSourceMove) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  sequence_.push_back(4);
  const SeriesIdSequenceSnapshot snapshot(sequence_);

  // Act
  auto moved = std::move(sequence_);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAre(1, 2, 4));
  EXPECT_THAT(moved, testing::ElementsAre(1, 2, 4));
  EXPECT_TRUE(sequence_.empty());
}

TEST_F(SeriesIdSequenceSnapshotFixture, SnapshotIteratorsAdvanceIndependently) {
  // Arrange
  sequence_.push_back(1);
  sequence_.push_back(2);
  const SeriesIdSequenceSnapshot snapshot(sequence_);
  auto first = snapshot.view().begin();
  const auto second = first;

  // Act
  ++first;

  // Assert
  EXPECT_EQ(2U, *first);
  EXPECT_EQ(1U, *second);
}

struct PostingSnapshotCase {
  const char* name;
  std::vector<uint32_t> values;
};
class PostingSnapshotBoundariesFixture : public testing::TestWithParam<PostingSnapshotCase> {
 protected:
  SeriesIdSequence source_;
};
TEST_P(PostingSnapshotBoundariesFixture, AppendAcrossKeyGroupsKeepsSnapshotValues) {
  // Arrange
  std::ranges::copy(GetParam().values, std::back_inserter(source_));
  const SeriesIdSequenceSnapshot snapshot(source_);

  // Act
  source_.push_back(10000);
  source_.push_back(20000);
  source_.push_back(40000);
  source_.push_back(80000);

  // Assert
  EXPECT_THAT(snapshot, testing::ElementsAreArray(GetParam().values));
  EXPECT_EQ(GetParam().values.size(), snapshot.count());
}
INSTANTIATE_TEST_SUITE_P(
    KeyGroups,
    PostingSnapshotBoundariesFixture,
    testing::Values(PostingSnapshotCase{.name = "Empty", .values = {}},
                    PostingSnapshotCase{.name = "Singleton", .values = {0U}},
                    PostingSnapshotCase{.name = "Pair", .values = {0U, 2U}},
                    PostingSnapshotCase{.name = "ThreeValues", .values = {0U, 2U, 6U}},
                    PostingSnapshotCase{.name = "FourValues", .values = {0U, 2U, 6U, 12U}},
                    PostingSnapshotCase{.name = "FiveValues", .values = {0U, 2U, 6U, 12U, 20U}},
                    PostingSnapshotCase{.name = "SixValues", .values = {0U, 2U, 6U, 12U, 20U, 30U}},
                    PostingSnapshotCase{.name = "SevenValues", .values = {0U, 2U, 6U, 12U, 20U, 30U, 42U}},
                    PostingSnapshotCase{.name = "EightValues", .values = {0U, 2U, 6U, 12U, 20U, 30U, 42U, 56U}},
                    PostingSnapshotCase{.name = "NineValues", .values = {0U, 2U, 6U, 12U, 20U, 30U, 42U, 56U, 72U}},
                    PostingSnapshotCase{.name = "SixteenValues", .values = {0U, 2U, 6U, 12U, 20U, 30U, 42U, 56U, 72U, 90U, 110U, 132U, 156U, 182U, 210U, 240U}},
                    PostingSnapshotCase{.name = "ThirtyTwoValues",
                                        .values = {0U,   2U,   6U,   12U,  20U,  30U,  42U,  56U,  72U,  90U,  110U, 132U, 156U, 182U, 210U, 240U,
                                                   272U, 306U, 342U, 380U, 420U, 462U, 506U, 552U, 600U, 650U, 702U, 756U, 812U, 870U, 930U, 992U}}),
    [](const testing::TestParamInfo<PostingSnapshotCase>& info) { return info.param.name; });

class LabelReverseIndexFixture : public testing::Test {
 protected:
  LabelReverseIndex index_;
};

TEST_F(LabelReverseIndexFixture, GetNonExistingLabelValue) {
  // Arrange

  // Act
  const auto item = index_.get(0);

  // Assert
  ASSERT_EQ(nullptr, item);
}

TEST_F(LabelReverseIndexFixture, AddIntoNewLabelValue) {
  // Arrange

  // Act
  index_.add(0, 0);
  const auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  EXPECT_THAT(*item, testing::ElementsAre(0U));
  EXPECT_THAT(*index_.get_all(), testing::ElementsAre(0U));
}

TEST_F(LabelReverseIndexFixture, AddIntoExistingLabelValue) {
  // Arrange

  // Act
  index_.add(0, 0);
  index_.add(0, 1);
  const auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  EXPECT_THAT(*item, testing::ElementsAre(0U, 1U));
  EXPECT_THAT(*index_.get_all(), testing::ElementsAre(0U, 1U));
}

TEST_F(LabelReverseIndexFixture, AddMultipleLabelValues) {
  // Arrange

  // Act
  index_.add(0, 0);
  index_.add(1, 1);
  const auto item0 = index_.get(0);
  const auto item1 = index_.get(1);

  // Assert
  ASSERT_NE(nullptr, item0);
  EXPECT_THAT(*item0, testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  EXPECT_THAT(*item1, testing::ElementsAre(1U));

  EXPECT_THAT(*index_.get_all(), testing::ElementsAre(0U, 1U));
}

TEST_F(LabelReverseIndexFixture, AddOutOfOrderLabelId) {
  // Arrange

  // Act
  index_.add(1, 1);
  const auto item0 = index_.get(0);
  const auto item1 = index_.get(1);

  // Assert
  ASSERT_NE(nullptr, item0);
  EXPECT_THAT(*item0, testing::ElementsAre());

  ASSERT_NE(nullptr, item1);
  EXPECT_THAT(*item1, testing::ElementsAre(1U));

  EXPECT_THAT(*index_.get_all(), testing::ElementsAre(1U));
}

class SeriesReverseIndexFixture : public testing::Test {
 protected:
  struct Label {
    uint32_t label_name_id;
    uint32_t label_value_id;

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return label_name_id; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return label_value_id; }
  };

  SeriesReverseIndex index_;
};

TEST_F(SeriesReverseIndexFixture, GetNonExistingLabelName) {
  // Arrange

  // Act
  const auto item = index_.get(0);

  // Assert
  EXPECT_EQ(nullptr, item);
}

TEST_F(SeriesReverseIndexFixture, AddIntoNewLabelName) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  const auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  EXPECT_THAT(*item, testing::ElementsAre(0U));
}

TEST_F(SeriesReverseIndexFixture, AddIntoExistingLabelName) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 1);
  const auto item = index_.get(0);

  // Assert
  ASSERT_NE(nullptr, item);
  EXPECT_THAT(*item, testing::ElementsAre(0U, 1U));
}

TEST_F(SeriesReverseIndexFixture, AddMultipleLabelNames) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 1, .label_value_id = 0}, 1);
  const auto item0 = index_.get(0);
  const auto item1 = index_.get(1);

  // Assert
  ASSERT_NE(nullptr, item0);
  EXPECT_THAT(*item0, testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  EXPECT_THAT(*item1, testing::ElementsAre(1U));
}

TEST_F(SeriesReverseIndexFixture, GetByNameAndValueId) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 0, .label_value_id = 0}, 0);
  index_.add(Label{.label_name_id = 0, .label_value_id = 1}, 1);
  const auto item0 = index_.get(0, 0);
  const auto item1 = index_.get(0, 1);
  const auto item2 = index_.get(0, 2);

  // Assert
  ASSERT_NE(nullptr, item0);
  EXPECT_THAT(*item0, testing::ElementsAre(0U));

  ASSERT_NE(nullptr, item1);
  EXPECT_THAT(*item1, testing::ElementsAre(1U));

  EXPECT_EQ(nullptr, item2);
}

TEST_F(SeriesReverseIndexFixture, AddOutOfOrderNameId) {
  // Arrange

  // Act
  index_.add(Label{.label_name_id = 1, .label_value_id = 0}, 0);
  const auto item0 = index_.get(0, 0);
  const auto item1 = index_.get(1, 0);

  // Assert
  ASSERT_EQ(nullptr, item0);

  ASSERT_NE(nullptr, item1);
  EXPECT_THAT(*item1, testing::ElementsAre(0U));
}

}  // namespace
