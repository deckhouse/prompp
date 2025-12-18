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

  // Act
  std::stringstream ss;
  checkpoint.save(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_TRUE(
      std::ranges::equal(decoding_table_, std::initializer_list{label_set1.names(), label_set2.names()}, [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(LabelNameSetDecodingTableTest, CreateViewFromEncodingBimap) {
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
    {
      auto composite = decoding_table_[0];
      ASSERT_EQ(2U, composite.size());
      auto it = composite.begin();
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("1", "1")), *it++);
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("2", "2")), *it);
    }
    {
      auto composite = decoding_table_[1];
      ASSERT_EQ(1U, composite.size());
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("3", "3")), *composite.begin());
    }
    {
      auto composite = decoding_table_[2];
      ASSERT_EQ(1U, composite.size());
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("4", "4")), *composite.begin());
    }
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
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> lss;
  std::stringstream stream;

  // Act
  lss.find_or_emplace(LabelViewSet{{"process", "php"}});
  lss.find_or_emplace(LabelViewSet{{"process", "nodejs"}});
  lss.find_or_emplace(LabelViewSet{{"process", "python"}});
  const auto checkpoint = lss.checkpoint();
  stream << checkpoint;
  stream >> encoding_table_;
  encoding_table_.shrink_to_checkpoint_size(encoding_table_.checkpoint());

  const auto nginx_id = lss.find_or_emplace(LabelViewSet{{"process", "nginx"}});
  const auto apache_id = lss.find_or_emplace(LabelViewSet{{"process", "apache"}});
  stream << lss.checkpoint() - checkpoint;
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

}  // namespace
