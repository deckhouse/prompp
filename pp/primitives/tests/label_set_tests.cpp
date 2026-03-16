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
  EXPECT_EQ(3U, view.label_name_sets().size());
  EXPECT_EQ(3U, view.keys().size());
  EXPECT_EQ(6U, view.values().size());
  EXPECT_TRUE(std::ranges::equal(view.keys(), std::initializer_list{"job", "pod", "run"}));
  EXPECT_TRUE(std::ranges::equal(view.values(), std::initializer_list{"1", "2", "3", "a", "b", "first"}));
}

TEST_F(LabelSetEncodingBimapTest, EncodingBimapViewCheckId) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  const auto id1 = encoding_table_.find_or_emplace(label_set1);
  const auto id2 = encoding_table_.find_or_emplace(label_set2);
  const auto id3 = encoding_table_.find_or_emplace(label_set3);

  const auto view = encoding_table_.data_view();

  // Act
  auto view_it = view.begin();

  const auto view_id1 = (view_it++).id();
  const auto view_id2 = (view_it++).id();
  const auto view_id3 = (view_it++).id();

  // Assert
  EXPECT_EQ(view_it, view.end());
  EXPECT_EQ(view_id1, id1);
  EXPECT_EQ(view_id2, id2);
  EXPECT_EQ(view_id3, id3);
}

TEST_F(LabelSetEncodingBimapTest, EncodingBimapViewCheckKeyId) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  // Act
  const auto view = encoding_table_.data_view();
  auto k_it = view.keys().begin();

  const auto job_id = (k_it++).id();
  const auto pod_id = (k_it++).id();
  const auto run_id = (k_it++).id();

  auto v_it = view.values().begin();

  const auto k1_id = (v_it++).key_id();
  const auto k2_id = (v_it++).key_id();
  const auto k3_id = (v_it++).key_id();
  const auto k4_id = (v_it++).key_id();
  const auto k5_id = (v_it++).key_id();
  const auto k6_id = (v_it++).key_id();

  // Assert
  EXPECT_EQ(k_it, view.keys().end());
  EXPECT_EQ(v_it, view.values().end());
  EXPECT_TRUE(std::ranges::equal(std::initializer_list{k1_id, k2_id, k3_id, k4_id, k5_id, k6_id},
                                 std::initializer_list{job_id, job_id, job_id, pod_id, pod_id, run_id}));
}

TEST_F(LabelSetEncodingBimapTest, ViewValuesForKeyId) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  const auto view = encoding_table_.data_view();
  auto k_it = view.keys().begin();
  const auto job_id = (k_it++).id();
  const auto pod_id = (k_it++).id();
  const auto run_id = (k_it++).id();

  // Act
  const auto job_values = view.values(job_id);
  const auto pod_values = view.values(pod_id);
  const auto run_values = view.values(run_id);

  // Assert
  EXPECT_EQ(3U, job_values.size());
  EXPECT_TRUE(std::ranges::equal(job_values, std::initializer_list{"1", "2", "3"}));

  EXPECT_EQ(2U, pod_values.size());
  EXPECT_TRUE(std::ranges::equal(pod_values, std::initializer_list{"a", "b"}));

  EXPECT_EQ(1U, run_values.size());
  EXPECT_TRUE(std::ranges::equal(run_values, std::initializer_list{"first"}));
}

TEST_F(LabelSetEncodingBimapTest, ViewKeySymbol) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  // Act
  const auto view = encoding_table_.data_view();
  auto k_it = view.keys().begin();

  const auto job_id = (k_it++).id();
  const auto pod_id = (k_it++).id();
  const auto run_id = (k_it++).id();

  // Assert
  EXPECT_EQ("job"sv, view.key_symbol(job_id));
  EXPECT_EQ("pod"sv, view.key_symbol(pod_id));
  EXPECT_EQ("run"sv, view.key_symbol(run_id));
}

TEST_F(LabelSetEncodingBimapTest, ViewValueSymbol) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};
  const LabelViewSet label_set3 = {{"job", "3"}, {"run", "first"}, {"pod", "b"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.find_or_emplace(label_set3);

  // Act
  const auto view = encoding_table_.data_view();
  auto v_it = view.values().begin();

  // Assert
  EXPECT_EQ("1"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
  ++v_it;
  EXPECT_EQ("2"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
  ++v_it;
  EXPECT_EQ("3"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
  ++v_it;
  EXPECT_EQ("a"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
  ++v_it;
  EXPECT_EQ("b"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
  ++v_it;
  EXPECT_EQ("first"sv, view.value_symbol(v_it.key_id(), v_it.value_id()));
}

TEST_F(LabelSetEncodingBimapTest, ViewSize) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};

  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  EXPECT_EQ(2U, view.size());
}

TEST_F(LabelSetEncodingBimapTest, ViewSizeAfterEmplace) {
  // Arrange
  const LabelViewSet label_set1 = {{"job", "1"}};
  const LabelViewSet label_set2 = {{"job", "2"}, {"pod", "a"}};

  encoding_table_.find_or_emplace(label_set1);
  const auto view1 = encoding_table_.data_view();

  // Act
  encoding_table_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(2U, view1.size());
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
  encoding_table_.save(ss, checkpoint);
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
  encoding_table_.save(ss, checkpoint);
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
  encoding_table_.save(ss, delta);
  const auto save_size = encoding_table_.save_size(delta);

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

  encoding_table_.save(ss, base_checkpoint);
  encoding_table_.save(ss, delta);

  // Act
  std::stringstream read_stream(ss.str());
  decoding_table_.load(read_stream);
  decoding_table_.load(read_stream);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_[id2]));
}

class LabelSetVersionMigrationTest : public testing::Test {
 protected:
  using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector>;
  using DecodingTable = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector>;
};

TEST_F(LabelSetVersionMigrationTest, Version1To2Migration) {
  // Arrange
  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}};

  EncodingBimap encoding_table_v1(1);
  const auto id1 = encoding_table_v1.find_or_emplace(label_set1);
  const auto base_checkpoint_v1 = encoding_table_v1.checkpoint();

  const auto id2 = encoding_table_v1.find_or_emplace(label_set2);
  const auto checkpoint_v1 = encoding_table_v1.checkpoint();
  const auto delta_v1 = checkpoint_v1 - base_checkpoint_v1;
  std::stringstream ss;
  encoding_table_v1.save(ss, base_checkpoint_v1);
  encoding_table_v1.save(ss, delta_v1);

  // Act
  DecodingTable decoding_table_v2(2);

  decoding_table_v2.load(ss);
  decoding_table_v2.load(ss);

  // Assert: Check all data and version
  EXPECT_EQ(1U, encoding_table_v1.version());
  EXPECT_EQ(2U, decoding_table_v2.version());

  EXPECT_EQ(2U, decoding_table_v2.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_v2[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_v2[id2]));
}

TEST_F(LabelSetVersionMigrationTest, Version2To1Migration) {
  // Arrange
  const LabelViewSet label_set1 = {{"name1", "value1"}};
  const LabelViewSet label_set2 = {{"name2", "value2"}};

  EncodingBimap encoding_table_v2(2);
  const auto id1 = encoding_table_v2.find_or_emplace(label_set1);
  const auto base_checkpoint_v2 = encoding_table_v2.checkpoint();

  const auto id2 = encoding_table_v2.find_or_emplace(label_set2);
  const auto checkpoint_v2 = encoding_table_v2.checkpoint();
  const auto delta_v2 = checkpoint_v2 - base_checkpoint_v2;

  std::stringstream ss;
  encoding_table_v2.save(ss, base_checkpoint_v2);
  encoding_table_v2.save(ss, delta_v2);

  // Act
  DecodingTable decoding_table_v1(1);

  decoding_table_v1.load(ss);
  decoding_table_v1.load(ss);

  // Assert
  EXPECT_EQ(2U, encoding_table_v2.version());
  EXPECT_EQ(1U, decoding_table_v1.version());

  EXPECT_EQ(2U, decoding_table_v1.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_v1[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_v1[id2]));
}

}  // namespace
