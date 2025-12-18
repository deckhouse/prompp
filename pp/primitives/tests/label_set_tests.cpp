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

class LabelSetEncodingBimapTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> encoding_table_;
};

TEST_F(LabelSetEncodingBimapTest, StoreAndRetrieveLabelSet) {
  // Arrange
  const LabelViewSet label_set = {{"name1", "value1"}, {"name2", "value2"}};

  // Act
  const auto id = encoding_table_.find_or_emplace(label_set);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  const auto retrieved = encoding_table_[id];
  EXPECT_TRUE(std::ranges::equal(label_set, retrieved));
}

TEST_F(LabelSetEncodingBimapTest, StoreMultipleLabelSets) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};

  // Act
  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto id2 = encoding_table_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(2U, encoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, encoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, encoding_table_[id2]));
}

TEST_F(LabelSetEncodingBimapTest, FindOrEmplaceReturnsSameIdForDuplicate) {
  // Arrange
  const LabelViewSet label_set = {{"duplicate", "set"}};

  // Act
  const auto id1 = encoding_table_.find_or_emplace(label_set);
  const auto id2 = encoding_table_.find_or_emplace(label_set);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_EQ(id1, id2);
}

TEST_F(LabelSetEncodingBimapTest, IterateOverLabelSets) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}, {"c", "3"}};

  // Act
  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(2U, encoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(encoding_table_, std::initializer_list{label_set1, label_set2}, [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(LabelSetEncodingBimapTest, CheckpointAndRollback) {
  // Arrange
  const LabelViewSet label_set1 = {{"before", "checkpoint"}};
  const LabelViewSet label_set2 = {{"after", "checkpoint"}};

  // Act
  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_EQ(id1, encoding_table_.find(label_set1).value());
  EXPECT_FALSE(encoding_table_.find(label_set2).has_value());
}

TEST_F(LabelSetEncodingBimapTest, CreateViewFromEncodingBimap) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  EXPECT_EQ(3U, view.size());
  EXPECT_EQ(3U, view.labels_keys().size());
  EXPECT_EQ(6U, view.labels_values().size());
  EXPECT_TRUE(std::ranges::equal(view.labels_keys(), std::initializer_list{"job", "pod", "run"}));
  EXPECT_TRUE(std::ranges::equal(view.labels_values(), std::initializer_list{"1", "2", "3", "a", "b", "first"}));
}

TEST_F(LabelSetEncodingBimapTest, EncodingBimapViewCheckId) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto id2 = encoding_table_.find_or_emplace(label_set2);
  const auto id3 = encoding_table_.find_or_emplace(label_set3);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  std::vector<uint32_t> view_ids;
  view_ids.reserve(view.size());
  for (auto it = view.begin(), e = view.end(); it != e; ++it) {
    view_ids.push_back(it.id());
  }
  EXPECT_TRUE(std::ranges::equal(view_ids, std::initializer_list{id1, id2, id3}));
}

TEST_F(LabelSetEncodingBimapTest, EncodingBimapViewCheckKeyId) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  const auto view = encoding_table_.data_view();

  auto k_it = view.labels_keys().begin();
  const auto job_id = (k_it++).id();
  const auto pod_id = (k_it++).id();
  const auto run_id = (k_it++).id();

  // Act
  std::vector<uint32_t> view_value_symbols_ids;
  for (auto it = view.labels_values().begin(), e = view.labels_values().end(); it != e; ++it) {
    view_value_symbols_ids.push_back(it.key_id());
  }

  // Assert
  EXPECT_TRUE(std::ranges::equal(view_value_symbols_ids, std::initializer_list{job_id, job_id, job_id, pod_id, pod_id, run_id}));
}

class LabelSetDecodingTableTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> decoding_table_;
};

TEST_F(LabelSetDecodingTableTest, LoadFromCheckpoint) {
  // Arrange
  const LabelViewSet label_set1 = {{"first", "1"}};
  const LabelViewSet label_set2 = {{"second", "2"}, {"third", "3"}};
  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto id2 = encoding_table_.find_or_emplace(label_set2);
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_[id2]));
}

TEST_F(LabelSetDecodingTableTest, IterateOverDecodingTable) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}};
  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(decoding_table_, std::initializer_list{label_set1, label_set2}, [](const auto& a, const auto& b) { return a == b; }));
}

class LabelSetDeltaCheckpointTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> decoding_table_;
};

TEST_F(LabelSetDeltaCheckpointTest, DeltaCheckpointSaveSize) {
  // Arrange
  BareBones::ShrinkedToFitOStringStream ss;
  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}, {"name3", "value3"}};

  encoding_table_.find_or_emplace(label_set1);
  const auto base_checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(label_set2);

  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  // Act
  ss << delta;
  const auto save_size = delta.save_size();

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

TEST_F(LabelSetDeltaCheckpointTest, LoadFromBaseCheckpointAndDelta) {
  // Arrange
  std::stringstream ss;
  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}, {"name3", "value3"}};

  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto base_checkpoint = encoding_table_.checkpoint();
  const auto id2 = encoding_table_.find_or_emplace(label_set2);
  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  base_checkpoint.save(ss);
  ss << delta;

  // Act
  std::stringstream read_stream(ss.str());
  decoding_table_.load(read_stream);
  decoding_table_.load(read_stream);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_[id2]));
}

}  // namespace
