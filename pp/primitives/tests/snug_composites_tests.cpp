#include <sstream>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "primitives/snug_composites_filaments.h"

namespace {

template <class T>
using SharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

template <class T>
using SharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;

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
  auto view_iter = encoding_table_.data_view().begin();

  // Assert
  EXPECT_EQ(id1, (view_iter++).id());
  EXPECT_EQ(id2, (view_iter++).id());
  EXPECT_EQ(id3, (view_iter++).id());
  EXPECT_EQ(id4, (view_iter++).id());
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
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Act & Assert
  EXPECT_EQ(2U, decoding_table_.size());
  auto it = decoding_table_.begin();
  EXPECT_TRUE(std::ranges::equal(label_set1.names(), *it++));
  EXPECT_TRUE(std::ranges::equal(label_set2.names(), *it++));
  EXPECT_EQ(decoding_table_.end(), it);
}

class LabelNameSetViewTest : public testing::Test {
 protected:
  using LabelNameSetEncodingBimap = PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<SharedVector>;
  using LabelNameSetDecodingTable = PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<SharedSpan>;
};

TEST_F(LabelNameSetViewTest, CreateViewFromEncodingBimap) {
  // Arrange
  LabelNameSetEncodingBimap source;
  const LabelViewSet source_label_set = {{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  source.find_or_emplace(source_label_set.names());

  // Act
  const LabelNameSetDecodingTable view(source);
  source.find_or_emplace(LabelViewSet{{"name4", "value4"}, {"name5", "value5"}}.names());

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_TRUE(std::ranges::equal(source_label_set.names(), view[0]));
}

TEST_F(LabelNameSetViewTest, ViewRemainsValidAfterSourceModification) {
  // Arrange
  auto source = std::make_unique<LabelNameSetEncodingBimap>();
  const LabelViewSet source_label_set = {{"persistent", "value"}};
  source->find_or_emplace(source_label_set.names());

  // Act
  const LabelNameSetDecodingTable view(*source);
  source.reset();

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_TRUE(std::ranges::equal(source_label_set.names(), view[0]));
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
  EXPECT_TRUE(std::ranges::equal(label_set, retrieved, [](const auto& a, const auto& b) { return a == b; }));
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
  EXPECT_TRUE(std::ranges::equal(label_set1, encoding_table_[id1], [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_TRUE(std::ranges::equal(label_set2, encoding_table_[id2], [](const auto& a, const auto& b) { return a == b; }));
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
  const LabelViewSet label_set2 = {{"b", "2"}};

  // Act
  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(2U, encoding_table_.size());
  auto it = encoding_table_.begin();
  EXPECT_TRUE(std::ranges::equal(label_set1, *it++, [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_TRUE(std::ranges::equal(label_set2, *it++, [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_EQ(encoding_table_.end(), it);
}

TEST_F(LabelSetEncodingBimapTest, CheckpointAndRollback) {
  // Arrange
  const LabelViewSet label_set1 = {{"before", "checkpoint"}};
  const LabelViewSet label_set2 = {{"after", "checkpoint"}};

  // Act
  encoding_table_.find_or_emplace(label_set1);
  const auto checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace(label_set2);
  encoding_table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(1U, encoding_table_.size());
  EXPECT_TRUE(std::ranges::equal(label_set1, *encoding_table_.begin(), [](const auto& a, const auto& b) { return a == b; }));
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
  EXPECT_TRUE(std::ranges::equal(label_set1, decoding_table_[id1], [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_TRUE(std::ranges::equal(label_set2, decoding_table_[id2], [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(LabelSetDecodingTableTest, IterateOverDecodingTable) {
  // Arrange
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}};
  encoding_table_.find_or_emplace(label_set1);
  encoding_table_.find_or_emplace(label_set2);
  const auto checkpoint = encoding_table_.checkpoint();
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Act & Assert
  EXPECT_EQ(2U, decoding_table_.size());
  auto it = decoding_table_.begin();
  EXPECT_TRUE(std::ranges::equal(label_set1, *it++, [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_TRUE(std::ranges::equal(label_set2, *it++, [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_EQ(decoding_table_.end(), it);
}

class LabelSetViewTest : public testing::Test {
 protected:
  using LabelSetEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<SharedVector>;
  using LabelSetDecodingTable = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpan>;
};

TEST_F(LabelSetViewTest, CreateViewFromEncodingBimap) {
  // Arrange
  LabelSetEncodingBimap source;
  const LabelViewSet source_data = {{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  source.find_or_emplace(source_data);

  // Act
  const LabelSetDecodingTable view(source);
  source.find_or_emplace(LabelViewSet{{"name4", "value4"}});

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_TRUE(std::ranges::equal(source_data, view[0], [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(LabelSetViewTest, ViewRemainsValidAfterSourceModification) {
  // Arrange
  auto source = std::make_unique<LabelSetEncodingBimap>();
  const LabelViewSet source_data = {{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  source->find_or_emplace(source_data);

  // Act
  const LabelSetDecodingTable view(*source);
  source.reset();

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_TRUE(std::ranges::equal(source_data, view[0], [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(LabelSetViewTest, AccessViewDataView) {
  // Arrange
  LabelSetEncodingBimap source;
  const LabelViewSet label_set1 = {{"a", "1"}};
  const LabelViewSet label_set2 = {{"b", "2"}};
  source.find_or_emplace(label_set1);
  source.find_or_emplace(label_set2);
  const LabelSetDecodingTable view(source);

  // Act
  const auto data_view = view.data_view();

  // Assert
  EXPECT_EQ(2U, data_view.size());
  auto it = data_view.begin();
  EXPECT_TRUE(std::ranges::equal(label_set1, *it++, [](const auto& a, const auto& b) { return a == b; }));
  EXPECT_TRUE(std::ranges::equal(label_set2, *it++, [](const auto& a, const auto& b) { return a == b; }));
}

class SymbolDeltaCheckpointTest : public testing::Test {
 protected:
  PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector> encoding_table_;
};

TEST_F(SymbolDeltaCheckpointTest, DeltaCheckpointSaveSize) {
  // Arrange
  encoding_table_.find_or_emplace("first"sv);
  const auto base_checkpoint = encoding_table_.checkpoint();
  encoding_table_.find_or_emplace("second"sv);
  const auto checkpoint = encoding_table_.checkpoint();
  const auto delta = checkpoint - base_checkpoint;
  BareBones::ShrinkedToFitOStringStream ss;

  // Act
  ss << delta;
  const auto save_size = delta.save_size();

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

}  // namespace
