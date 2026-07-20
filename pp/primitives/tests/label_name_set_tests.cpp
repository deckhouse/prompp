#include <sstream>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"

namespace {
using BareBones::Vector;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;
using std::string_literals::operator""s;

class LabelNameSetEncodingBimapTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector> encoding_table_;
};

TEST_F(LabelNameSetEncodingBimapTest, StoreAndRetrieveLabelNameSet) {
  // Arrange
  const LabelViewSet label_set = {{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};

  // Act
  const auto id = encoding_table_.find_or_emplace(label_set.names());

  // Assert
  EXPECT_EQ(1U, encoding_table_.items_count());
  const auto retrieved = encoding_table_[id];
  EXPECT_TRUE(std::ranges::equal(label_set.names(), retrieved));
}

TEST_F(LabelNameSetEncodingBimapTest, StoreMultipleLabelNameSets) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}, {"b", "2"}};
  const LabelViewSet label_set2 = {{"c", "3"}, {"d", "4"}, {"e", "5"}};

  // Act
  const auto id1 = encoding_table_.find_or_emplace(label_set1.names());
  const auto id2 = encoding_table_.find_or_emplace(label_set2.names());

  // Assert
  EXPECT_EQ(2U, encoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), encoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), encoding_table_[id2]));
}

TEST_F(LabelNameSetEncodingBimapTest, FindOrEmplaceReturnsSameIdForDuplicate) {
  // Arrange
  const LabelViewSet label_set = {{"duplicate", "value1"}, {"set", "value2"}};

  // Act
  const auto id1 = encoding_table_.find_or_emplace(label_set.names());
  const auto id2 = encoding_table_.find_or_emplace(label_set.names());

  // Assert
  EXPECT_EQ(1U, encoding_table_.items_count());
  EXPECT_EQ(id1, id2);
}

TEST_F(LabelNameSetEncodingBimapTest, IterateOverLabelNameSets) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};

  // Act
  encoding_table_.find_or_emplace(label_set1.names());
  encoding_table_.find_or_emplace(label_set2.names());

  // Assert
  EXPECT_EQ(2U, encoding_table_.items_count());
  auto it = encoding_table_.begin();
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), *it++));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), *it++));
  EXPECT_EQ(encoding_table_.end(), it);
}

TEST_F(LabelNameSetEncodingBimapTest, CheckpointAndRollback) {
  // Arrange
  const LabelViewSet label_set1 = {{"before", "checkpoint"}};
  const LabelViewSet label_set2 = {{"after", "checkpoint"}};

  // Act
  encoding_table_.find_or_emplace(label_set1.names());
  const auto checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(label_set2.names());
  encoding_table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(1U, encoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), *encoding_table_.begin()));
}

TEST_F(LabelNameSetEncodingBimapTest, CreateViewFromEncodingBimap) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};
  const LabelViewSet label_set3 = {{"d", "4"}, {"e", "5"}, {"f", "6"}};
  encoding_table_.find_or_emplace(label_set1.names());
  encoding_table_.find_or_emplace(label_set2.names());
  encoding_table_.find_or_emplace(label_set3.names());

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  EXPECT_EQ(encoding_table_.items_count(), view.size());
  EXPECT_TRUE(std::ranges::equal(encoding_table_, view));
}

TEST_F(LabelNameSetEncodingBimapTest, CreateViewSymbolsFromEncodingBimap) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};
  const LabelViewSet label_set3 = {{"d", "4"}, {"e", "5"}, {"f", "6"}};
  encoding_table_.find_or_emplace(label_set1.names());
  encoding_table_.find_or_emplace(label_set2.names());
  encoding_table_.find_or_emplace(label_set3.names());

  // Act
  const auto symbols = encoding_table_.data_view().symbols();

  // Assert
  EXPECT_EQ(6U, symbols.size());
  EXPECT_TRUE(std::ranges::equal(symbols, std::initializer_list{"a", "b", "c", "d", "e", "f"}));
}

TEST_F(LabelNameSetEncodingBimapTest, EncodingBimapViewIteratorId) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};
  const LabelViewSet label_set3 = {{"d", "4"}, {"e", "5"}, {"f", "6"}};

  const auto id1 = encoding_table_.find_or_emplace(label_set1.names());
  const auto id2 = encoding_table_.find_or_emplace(label_set2.names());
  const auto id3 = encoding_table_.find_or_emplace(label_set3.names());

  const auto view = encoding_table_.data_view();

  // Act
  auto view_it = view.begin();

  const auto view_id1 = (view_it++).id();
  const auto view_id2 = (view_it++).id();
  const auto view_id3 = (view_it++).id();

  // Assert
  EXPECT_EQ(view_it, view.end());
  EXPECT_TRUE(std::ranges::equal(std::initializer_list{view_id1, view_id2, view_id3}, std::initializer_list{id1, id2, id3}));
}

class LabelNameSetDecodingTableTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<Vector> decoding_table_;
};

TEST_F(LabelNameSetDecodingTableTest, LoadFromCheckpoint) {
  // Arrange
  const LabelViewSet label_set1 = {{"first", "1"}};
  const LabelViewSet label_set2 = {{"second", "2"}, {"third", "3"}};
  const auto id1 = encoding_table_.find_or_emplace(label_set1.names());
  const auto id2 = encoding_table_.find_or_emplace(label_set2.names());
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), decoding_table_[id2]));
}

TEST_F(LabelNameSetDecodingTableTest, IterateOverDecodingTable) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};
  encoding_table_.find_or_emplace(label_set1.names());
  encoding_table_.find_or_emplace(label_set2.names());
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
  EXPECT_TRUE(
      std::ranges::equal(decoding_table_, std::initializer_list{label_set1.names(), label_set2.names()}, [](const auto& a, const auto& b) { return a == b; }));
}

class LabelNameSetDeltaCheckpointTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<Vector> decoding_table_;
};

TEST_F(LabelNameSetDeltaCheckpointTest, DeltaCheckpointSaveSize) {
  // Arrange
  BareBones::ShrinkedToFitOStringStream ss;

  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}, {"name3", "value3"}};

  encoding_table_.find_or_emplace(label_set1.names());
  const auto base_checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(label_set2.names());
  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  // Act
  encoding_table_.save(ss, delta);
  const auto save_size = encoding_table_.save_size(delta);

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

TEST_F(LabelNameSetDeltaCheckpointTest, LoadFromBaseCheckpointAndDelta) {
  // Arrange
  std::stringstream ss;
  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}, {"name3", "value3"}};
  const auto id1 = encoding_table_.find_or_emplace(label_set1.names());
  const auto base_checkpoint = encoding_table_.checkpoint();
  const auto id2 = encoding_table_.find_or_emplace(label_set2.names());
  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  encoding_table_.save(ss, base_checkpoint);
  encoding_table_.save(ss, delta);

  // Act
  decoding_table_.load(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), decoding_table_[id2]));
}

}  // namespace
