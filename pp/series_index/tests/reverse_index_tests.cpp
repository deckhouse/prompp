#include <gmock/gmock.h>

#include "series_index/reverse_index.h"

namespace {

using series_index::LabelReverseIndex;
using series_index::SeriesReverseIndex;

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
