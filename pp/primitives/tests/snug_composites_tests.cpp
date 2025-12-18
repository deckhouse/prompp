#include <sstream>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "primitives/snug_composites_filaments.h"

namespace {
using BareBones::Vector;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;
using std::string_literals::operator""s;

class SymbolEncodingBimapTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector> encoding_table_;
};

TEST_F(SymbolEncodingBimapTest, StoreAndRetrieveSymbol) {
  // Arrange
  const std::string symbol = "test_symbol";

  // Act
  const auto id = encoding_table_.find_or_emplace(symbol);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_EQ(symbol, encoding_table_[id]);
  EXPECT_EQ(id, encoding_table_.find(symbol));
}

TEST_F(SymbolEncodingBimapTest, StoreMultipleSymbols) {
  // Arrange
  const std::string symbol1 = "first";
  const std::string symbol2 = "second";
  const std::string symbol3 = "third";

  // Act
  const auto id1 = encoding_table_.find_or_emplace(symbol1);
  const auto id2 = encoding_table_.find_or_emplace(symbol2);
  const auto id3 = encoding_table_.find_or_emplace(symbol3);

  // Assert
  EXPECT_EQ(3U, encoding_table_.size());

  EXPECT_EQ(symbol1, encoding_table_[id1]);
  EXPECT_EQ(symbol2, encoding_table_[id2]);
  EXPECT_EQ(symbol3, encoding_table_[id3]);

  EXPECT_EQ(id1, encoding_table_.find(symbol1));
  EXPECT_EQ(id2, encoding_table_.find(symbol2));
  EXPECT_EQ(id3, encoding_table_.find(symbol3));
}

TEST_F(SymbolEncodingBimapTest, FindOrEmplaceReturnsSameIdForDuplicate) {
  // Arrange
  const std::string symbol = "duplicate";

  // Act
  const auto id1 = encoding_table_.find_or_emplace(symbol);
  const auto id2 = encoding_table_.find_or_emplace(symbol);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_EQ(id1, id2);
}

TEST_F(SymbolEncodingBimapTest, IterateOverSymbols) {
  // Arrange
  const std::string symbol1 = "a";
  const std::string symbol2 = "b";
  const std::string symbol3 = "c";

  // Act
  encoding_table_.find_or_emplace(symbol1);
  encoding_table_.find_or_emplace(symbol2);
  encoding_table_.find_or_emplace(symbol3);

  // Assert
  EXPECT_EQ(3U, encoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(encoding_table_, std::initializer_list{"a"s, "b"s, "c"s}));
}

TEST_F(SymbolEncodingBimapTest, CheckpointAndRollback) {
  // Arrange
  const std::string symbol1 = "before_checkpoint";
  const std::string symbol2 = "after_checkpoint";

  // Act
  encoding_table_.find_or_emplace(symbol1);
  const auto checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(symbol2);
  encoding_table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_TRUE(encoding_table_.find(symbol1).has_value());
  EXPECT_FALSE(encoding_table_.find(symbol2).has_value());
}

TEST_F(SymbolEncodingBimapTest, CreateViewFromEncodingBimap) {
  // Arrange
  encoding_table_.find_or_emplace("lol"s);
  encoding_table_.find_or_emplace("kek"s);
  encoding_table_.find_or_emplace("pod"s);
  encoding_table_.find_or_emplace("job"s);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  EXPECT_EQ(4U, view.size());
  EXPECT_TRUE(std::ranges::equal(view, encoding_table_));
}

TEST_F(SymbolEncodingBimapTest, EncodingBimapViewIteratorId) {
  // Arrange
  const auto id1 = encoding_table_.find_or_emplace("lol"s);
  const auto id2 = encoding_table_.find_or_emplace("kek"s);
  const auto id3 = encoding_table_.find_or_emplace("pod"s);
  const auto id4 = encoding_table_.find_or_emplace("job"s);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  std::vector<uint32_t> view_ids;
  view_ids.reserve(view.size());
  for (auto it = view.begin(), e = view.end(); it != e; ++it) {
    view_ids.push_back(it.id());
  }
  EXPECT_TRUE(std::ranges::equal(view_ids, std::initializer_list{id1, id2, id3, id4}));
}

class SymbolDecodingTableTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::Symbol::DecodingTable<Vector> decoding_table_;
};

TEST_F(SymbolDecodingTableTest, LoadFromCheckpoint) {
  // Arrange
  const std::string symbol1 = "first";
  const std::string symbol2 = "second";
  const auto id1 = encoding_table_.find_or_emplace(symbol1);
  const auto id2 = encoding_table_.find_or_emplace(symbol2);
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_EQ(symbol1, decoding_table_[id1]);
  EXPECT_EQ(symbol2, decoding_table_[id2]);
}

TEST_F(SymbolDecodingTableTest, IterateOverDecodingTable) {
  // Arrange
  const std::string symbol1 = "a";
  const std::string symbol2 = "b";
  encoding_table_.find_or_emplace(symbol1);
  encoding_table_.find_or_emplace(symbol2);
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(decoding_table_, std::initializer_list{"a"s, "b"s}));
}

TEST_F(SymbolDecodingTableTest, CheckpointSaveSizeMatchesActualSize) {
  // Arrange
  encoding_table_.find_or_emplace("test"sv);
  const auto checkpoint = encoding_table_.checkpoint();
  BareBones::ShrinkedToFitOStringStream ss;

  // Act
  checkpoint.save(ss);
  const auto save_size = checkpoint.save_size();

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

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
  EXPECT_EQ(1U, encoding_table_.size());
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
  EXPECT_EQ(2U, encoding_table_.size());
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
  EXPECT_EQ(1U, encoding_table_.size());
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
  EXPECT_EQ(2U, encoding_table_.size());
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
  EXPECT_EQ(1U, encoding_table_.size());
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
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
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
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(
      std::ranges::equal(decoding_table_, std::initializer_list{label_set1.names(), label_set2.names()}, [](const auto& a, const auto& b) { return a == b; }));
}

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

class SymbolDeltaCheckpointTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::Symbol::DecodingTable<Vector> decoding_table_;
};

TEST_F(SymbolDeltaCheckpointTest, DeltaCheckpointSaveSize) {
  // Arrange
  BareBones::ShrinkedToFitOStringStream ss;

  encoding_table_.find_or_emplace("first"sv);
  const auto base_checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace("second"sv);

  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  // Act
  ss << delta;
  const auto save_size = delta.save_size();

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

TEST_F(SymbolDeltaCheckpointTest, LoadFromBaseCheckpointAndDelta) {
  // Arrange
  std::stringstream ss;
  const std::string symbol1 = "first";
  const std::string symbol2 = "second";

  const auto id1 = encoding_table_.find_or_emplace(symbol1);
  const auto base_checkpoint = encoding_table_.checkpoint();
  const auto id2 = encoding_table_.find_or_emplace(symbol2);
  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;

  base_checkpoint.save(ss);
  ss << delta;

  // Act
  decoding_table_.load(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_EQ(symbol1, decoding_table_[id1]);
  EXPECT_EQ(symbol2, decoding_table_[id2]);
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
  ss << delta;
  const auto save_size = delta.save_size();

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

  base_checkpoint.save(ss);
  ss << delta;

  // Act
  decoding_table_.load(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), decoding_table_[id1]));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), decoding_table_[id2]));
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

class ShrinkableEncodingBimapLabelSetFixture : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<Vector> encoding_table_;
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> decoding_table_;
  std::array<LabelViewSet, 6> ls_;

  void SetUp() override {
    ls_[0] = {{"1", "1"}, {"2", "2"}};
    ls_[1] = {{"3", "3"}};
    ls_[2] = {{"4", "4"}};
    ls_[3] = {{"5", "5"}};
    ls_[4] = {{"6", "6"}};
    ls_[5] = {{"7", "7"}};
  }

  auto create_and_load_checkpoint(const typename PromPP::Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<Vector>::checkpoint_type* from) {
    auto checkpoint = encoding_table_.checkpoint();
    std::stringstream ss;
    checkpoint.save(ss, from);
    decoding_table_.load(ss);
    return checkpoint;
  }

  void check_decoding_table() const {
    ASSERT_EQ(3U, decoding_table_.size());
    const LabelViewSet expected_label_set0{{"1", "1"}, {"2", "2"}};
    const LabelViewSet expected_label_set1{{"3", "3"}};
    const LabelViewSet expected_label_set2{{"4", "4"}};
    EXPECT_TRUE(std::ranges::equal(expected_label_set0, decoding_table_[0]));
    EXPECT_TRUE(std::ranges::equal(expected_label_set1, decoding_table_[1]));
    EXPECT_TRUE(std::ranges::equal(expected_label_set2, decoding_table_[2]));
  }
};

TEST_F(ShrinkableEncodingBimapLabelSetFixture, ShrinkAndLoad) {
  // Arrange

  // Act
  {
    encoding_table_.find_or_emplace(ls_[0]);
    encoding_table_.find_or_emplace(ls_[1]);
    const auto checkpoint = create_and_load_checkpoint(nullptr);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }
  {
    const auto empty_checkpoint = encoding_table_.checkpoint();
    encoding_table_.find_or_emplace(ls_[2]);
    const auto checkpoint = create_and_load_checkpoint(&empty_checkpoint);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, LoadWithoutShrink) {
  // Arrange

  // Act
  {
    encoding_table_.find_or_emplace(ls_[0]);
    encoding_table_.find_or_emplace(ls_[1]);
    create_and_load_checkpoint(nullptr);
  }
  {
    const auto empty_checkpoint = encoding_table_.checkpoint();
    encoding_table_.find_or_emplace(ls_[2]);
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, LoadFromNonShrinkableTable) {
  // Arrange
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> non_shrinkable_encoding_bimap;
  std::stringstream stream;

  // Act
  non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "php"}});
  non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "nodejs"}});
  non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "python"}});
  const auto checkpoint = non_shrinkable_encoding_bimap.checkpoint();
  stream << checkpoint;
  stream >> encoding_table_;
  encoding_table_.shrink_to_checkpoint_size(encoding_table_.checkpoint());

  const auto nginx_id = non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "nginx"}});
  const auto apache_id = non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "apache"}});
  stream << non_shrinkable_encoding_bimap.checkpoint() - checkpoint;
  stream >> encoding_table_;

  // Assert
  EXPECT_FALSE(encoding_table_.find(LabelViewSet{{"process", "php"}}).has_value());
  EXPECT_EQ(nginx_id, encoding_table_.find(LabelViewSet{{"process", "nginx"}}).value());
  EXPECT_EQ(apache_id, encoding_table_.find(LabelViewSet{{"process", "apache"}}).value());
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, EmptyCheckpointWithShrink) {
  // Arrange

  // Act
  {
    encoding_table_.find_or_emplace(ls_[0]);
    encoding_table_.find_or_emplace(ls_[1]);
    encoding_table_.find_or_emplace(ls_[2]);
    const auto checkpoint = create_and_load_checkpoint(nullptr);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }
  {
    const auto empty_checkpoint = encoding_table_.checkpoint();
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, EmptyCheckpointWithoutShrink) {
  // Arrange

  // Act
  {
    encoding_table_.find_or_emplace(ls_[0]);
    encoding_table_.find_or_emplace(ls_[1]);
    encoding_table_.find_or_emplace(ls_[2]);
    create_and_load_checkpoint(nullptr);
  }
  {
    const auto empty_checkpoint = encoding_table_.checkpoint();
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, CheckSaveSize) {
  // Arrange
  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);

  auto checkpoint = encoding_table_.checkpoint();

  encoding_table_.shrink_to_checkpoint_size(checkpoint);

  encoding_table_.find_or_emplace(ls_[1]);
  encoding_table_.find_or_emplace(ls_[2]);
  encoding_table_.find_or_emplace(ls_[3]);
  encoding_table_.find_or_emplace(ls_[4]);
  encoding_table_.find_or_emplace(ls_[5]);

  auto checkpoint2 = encoding_table_.checkpoint();

  auto delta = checkpoint2 - checkpoint;
  BareBones::ShrinkedToFitOStringStream ss;
  ss << delta;

  // Act
  const auto save_size = delta.save_size();

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

class SharedDataFixture : public testing::Test {
 protected:
  template <class T>
  using SharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;

  template <class T>
  using SharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

  using SymbolEncodingBimap = PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<SharedVector>;
  using SymbolDecodingTable = PromPP::Primitives::SnugComposites::Symbol::DecodingTable<SharedSpan>;

  using LabelNameSetEncodingBimap = PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<SharedVector>;
  using LabelNameSetDecodingTable = PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<SharedSpan>;

  using LabelSetEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<SharedVector>;
  using LabelSetDecodingTable = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpan>;
};

TEST_F(SharedDataFixture, CopySymbol) {
  // Arrange
  SymbolEncodingBimap encoding_bimap;
  constexpr auto symbol = "string1"sv;
  encoding_bimap.find_or_emplace(symbol);

  // Act
  const SymbolDecodingTable decoding_table(encoding_bimap);
  encoding_bimap.find_or_emplace("string2"sv);

  // Assert
  EXPECT_EQ(1U, decoding_table.size());
  EXPECT_EQ(symbol, decoding_table[0]);
}

TEST_F(SharedDataFixture, CopyLabelNameSet) {
  // Arrange
  LabelNameSetEncodingBimap encoding_bimap;
  const LabelViewSet label_set{{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  encoding_bimap.find_or_emplace(label_set.names());

  // Act
  const LabelNameSetDecodingTable decoding_table(encoding_bimap);
  encoding_bimap.find_or_emplace(LabelViewSet{{"name4", "value4"}}.names());

  // Assert
  EXPECT_EQ(1U, decoding_table.size());
  EXPECT_TRUE(std::ranges::equal(label_set.names(), decoding_table[0]));
}

TEST_F(SharedDataFixture, CopyLabelSet) {
  // Arrange
  LabelSetEncodingBimap encoding_bimap;
  const LabelViewSet label_set{{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  encoding_bimap.find_or_emplace(label_set);

  // Act
  const LabelSetDecodingTable decoding_table(encoding_bimap);
  encoding_bimap.find_or_emplace(LabelViewSet{{"name4", "value4"}});

  // Assert
  EXPECT_EQ(1U, decoding_table.size());
  EXPECT_TRUE(std::ranges::equal(label_set, decoding_table[0]));
}

TEST_F(SharedDataFixture, UseCopyLabelSetAfterFreeSourceLabelSet) {
  // Arrange
  auto encoding_bimap = std::make_unique<LabelSetEncodingBimap>();
  const LabelViewSet label_set{{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  encoding_bimap->find_or_emplace(label_set);

  // Act
  const LabelSetDecodingTable decoding_table(*encoding_bimap);
  encoding_bimap.reset();

  // Assert
  EXPECT_EQ(1U, decoding_table.size());
  EXPECT_TRUE(std::ranges::equal(label_set, decoding_table[0]));
}

}  // namespace
