#include <sstream>
#include <string>
#include <vector>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "bare_bones/vector.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"

namespace {

template <class T>
using SharedSpan = BareBones::SharedSpan<T, BareBones::SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

static_assert(std::same_as<PromPP::Primitives::SnugComposites::Symbol::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<BareBones::Vector>::value_type>);
static_assert(std::same_as<PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<BareBones::Vector>::value_type>);
static_assert(std::same_as<PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>::value_type>);

static_assert(std::same_as<PromPP::Primitives::SnugComposites::Symbol::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::Symbol::DecodingTable<SharedSpan>::value_type>);
static_assert(std::same_as<PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<SharedSpan>::value_type>);
static_assert(std::same_as<PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::Vector>::value_type,
                           PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpan>::value_type>);

using BareBones::Vector;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;
using std::operator""s;

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
  EXPECT_EQ(1U, encoding_table_.items_count());
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
  EXPECT_EQ(3U, encoding_table_.items_count());

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
  EXPECT_EQ(1U, encoding_table_.items_count());
  EXPECT_EQ(id1, id2);
}

TEST_F(SymbolEncodingBimapTest, IterateOverEmptySymbols) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, encoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(encoding_table_, std::initializer_list<std::string>{}));
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
  EXPECT_EQ(3U, encoding_table_.items_count());
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
  EXPECT_EQ(1U, encoding_table_.items_count());
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

  const auto view = encoding_table_.data_view();

  // Act

  auto view_it = view.begin();

  const auto view_id1 = (view_it++).id();
  const auto view_id2 = (view_it++).id();
  const auto view_id3 = (view_it++).id();
  const auto view_id4 = (view_it++).id();

  // Assert
  EXPECT_EQ(view_it, view.end());
  EXPECT_TRUE(std::ranges::equal(std::initializer_list{view_id1, view_id2, view_id3, view_id4}, std::initializer_list{id1, id2, id3, id4}));
}

TEST_F(SymbolEncodingBimapTest, ViewIndexOperator) {
  // Arrange
  const auto id1 = encoding_table_.find_or_emplace("lol"s);
  const auto id2 = encoding_table_.find_or_emplace("kek"s);
  const auto id3 = encoding_table_.find_or_emplace("pod"s);

  // Act
  const auto view = encoding_table_.data_view();

  // Assert
  EXPECT_EQ("lol"sv, view[id1]);
  EXPECT_EQ("kek"sv, view[id2]);
  EXPECT_EQ("pod"sv, view[id3]);
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
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
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
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
  EXPECT_TRUE(std::ranges::equal(decoding_table_, std::initializer_list{"a"s, "b"s}));
}

TEST_F(SymbolDecodingTableTest, SymbolTableReadViewMatchesOperatorBracketAfterLoad) {
  // Arrange
  encoding_table_.find_or_emplace("a"sv);
  encoding_table_.find_or_emplace("bb"sv);
  encoding_table_.find_or_emplace("ccc"sv);
  const auto checkpoint = encoding_table_.checkpoint();
  std::stringstream ss;
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Act
  const auto& symbol_read_view = decoding_table_.symbol_table_read_view();

  // Assert
  EXPECT_EQ(decoding_table_[0], symbol_read_view[0]);
  EXPECT_EQ(decoding_table_[1], symbol_read_view[1]);
  EXPECT_EQ(decoding_table_[2], symbol_read_view[2]);
}

TEST_F(SymbolDecodingTableTest, CheckpointSaveSizeMatchesActualSize) {
  // Arrange
  encoding_table_.find_or_emplace("test"sv);
  const auto checkpoint = encoding_table_.checkpoint();
  BareBones::ShrinkedToFitOStringStream ss;

  // Act
  encoding_table_.save(ss, checkpoint);
  const auto save_size = encoding_table_.save_size(checkpoint);

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
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
  encoding_table_.save(ss, delta);
  const auto save_size = encoding_table_.save_size(delta);

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

  encoding_table_.save(ss, base_checkpoint);
  encoding_table_.save(ss, delta);

  // Act
  decoding_table_.load(ss);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.items_count());
  EXPECT_EQ(symbol1, decoding_table_[id1]);
  EXPECT_EQ(symbol2, decoding_table_[id2]);
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

  auto create_and_load_checkpoint(const PromPP::Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<Vector>::checkpoint_type* from) {
    auto checkpoint = encoding_table_.checkpoint();
    std::stringstream ss;
    encoding_table_.save(ss, checkpoint, from);
    decoding_table_.load(ss);
    return checkpoint;
  }

  void check_decoding_table() const {
    ASSERT_EQ(3U, decoding_table_.items_count());
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
  non_shrinkable_encoding_bimap.save(stream, non_shrinkable_encoding_bimap.checkpoint());
  stream >> encoding_table_;
  encoding_table_.shrink_to_checkpoint_size(encoding_table_.checkpoint());

  const auto nginx_id = non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "nginx"}});
  const auto apache_id = non_shrinkable_encoding_bimap.find_or_emplace(LabelViewSet{{"process", "apache"}});
  non_shrinkable_encoding_bimap.save(stream, non_shrinkable_encoding_bimap.checkpoint() - checkpoint);
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
  encoding_table_.save(ss, delta);

  // Act
  const auto save_size = encoding_table_.save_size(delta);

  // Assert
  EXPECT_EQ(ss.view().size(), save_size);
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, ShrunkElementsRemoved) {
  // Arrange:
  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);
  const auto checkpoint = encoding_table_.checkpoint();

  // Act
  encoding_table_.shrink_to_checkpoint_size(checkpoint);

  // Assert
  EXPECT_FALSE(encoding_table_.find(ls_[0]).has_value());
  EXPECT_FALSE(encoding_table_.find(ls_[1]).has_value());
  EXPECT_EQ(0U, encoding_table_.items_count());
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, NonShrunkElementsRemainingAccessible) {
  // Arrange
  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);
  const auto checkpoint = encoding_table_.checkpoint();
  [[maybe_unused]] const auto id2 = encoding_table_.find_or_emplace(ls_[2]);
  [[maybe_unused]] const auto id3 = encoding_table_.find_or_emplace(ls_[3]);

  // Act
  encoding_table_.shrink_to_checkpoint_size(checkpoint);

  // Assert
  EXPECT_FALSE(encoding_table_.find(ls_[0]).has_value());
  EXPECT_FALSE(encoding_table_.find(ls_[1]).has_value());
  EXPECT_TRUE(encoding_table_.find(ls_[2]).has_value());
  EXPECT_TRUE(encoding_table_.find(ls_[3]).has_value());

  EXPECT_EQ(2U, encoding_table_.items_count());
  EXPECT_EQ(id2, encoding_table_.find(ls_[2]).value());
  EXPECT_EQ(id3, encoding_table_.find(ls_[3]).value());
  EXPECT_TRUE(std::ranges::equal(ls_[2], encoding_table_[id2]));
  EXPECT_TRUE(std::ranges::equal(ls_[3], encoding_table_[id3]));
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, AddedElementsAfterShrinkRemainingAccessible) {
  // Arrange
  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);
  const auto checkpoint = encoding_table_.checkpoint();
  encoding_table_.shrink_to_checkpoint_size(checkpoint);

  // Act
  const auto id2 = encoding_table_.find_or_emplace(ls_[2]);
  const auto id3 = encoding_table_.find_or_emplace(ls_[3]);

  // Assert
  EXPECT_EQ(2U, encoding_table_.items_count());
  EXPECT_EQ(id2, encoding_table_.find(ls_[2]).value());
  EXPECT_EQ(id3, encoding_table_.find(ls_[3]).value());
  EXPECT_TRUE(std::ranges::equal(ls_[2], encoding_table_[id2]));
  EXPECT_TRUE(std::ranges::equal(ls_[3], encoding_table_[id3]));
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, FullCheckpointChainSaveAndLoadAllDataRestored) {
  // Arrange
  std::stringstream snapshot_stream;
  std::stringstream delta1_stream;
  std::stringstream delta2_stream;

  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);
  const auto checkpoint1 = encoding_table_.checkpoint();
  encoding_table_.save(snapshot_stream, checkpoint1);

  encoding_table_.find_or_emplace(ls_[2]);
  encoding_table_.find_or_emplace(ls_[3]);
  const auto checkpoint2 = encoding_table_.checkpoint();
  encoding_table_.save(delta1_stream, checkpoint2 - checkpoint1);

  encoding_table_.shrink_to_checkpoint_size(checkpoint2);
  const auto checkpoint_after_shrink = encoding_table_.checkpoint();

  encoding_table_.find_or_emplace(ls_[4]);
  encoding_table_.find_or_emplace(ls_[5]);
  const auto checkpoint3 = encoding_table_.checkpoint();
  encoding_table_.save(delta2_stream, checkpoint3 - checkpoint_after_shrink);

  // Act
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> loaded_table;
  snapshot_stream >> loaded_table;
  delta1_stream >> loaded_table;
  delta2_stream >> loaded_table;

  // Assert
  EXPECT_EQ(6U, loaded_table.items_count());
  EXPECT_TRUE(std::ranges::equal(ls_, loaded_table, [](const auto& a, const auto& b) { return a == b; }));
}

TEST_F(ShrinkableEncodingBimapLabelSetFixture, FullCheckpointChainWithPartialShrink_SaveAndLoad_AllDataRestored) {
  // Arrange
  std::stringstream snapshot_stream;
  std::stringstream delta1_stream;
  std::stringstream delta2_stream;

  encoding_table_.find_or_emplace(ls_[0]);
  encoding_table_.find_or_emplace(ls_[1]);
  const auto checkpoint1 = encoding_table_.checkpoint();
  encoding_table_.save(snapshot_stream, checkpoint1);

  encoding_table_.find_or_emplace(ls_[2]);
  encoding_table_.find_or_emplace(ls_[3]);
  const auto checkpoint2 = encoding_table_.checkpoint();
  encoding_table_.save(delta1_stream, checkpoint2 - checkpoint1);

  encoding_table_.shrink_to_checkpoint_size(checkpoint1);
  const auto checkpoint_after_shrink = encoding_table_.checkpoint();

  encoding_table_.find_or_emplace(ls_[4]);
  encoding_table_.find_or_emplace(ls_[5]);
  const auto checkpoint3 = encoding_table_.checkpoint();
  encoding_table_.save(delta2_stream, checkpoint3 - checkpoint_after_shrink);

  // Act
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> loaded_table;
  snapshot_stream >> loaded_table;
  delta1_stream >> loaded_table;
  delta2_stream >> loaded_table;

  // Assert
  EXPECT_EQ(6U, loaded_table.items_count());
  EXPECT_TRUE(std::ranges::equal(ls_, loaded_table, [](const auto& a, const auto& b) { return a == b; }));
}

class SharedDataFixture : public testing::Test {
 protected:
  template <class T>
  using SharedVector = BareBones::SharedVector<T, BareBones::SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

  template <class T>
  using SharedSpan = BareBones::SharedSpan<T, BareBones::SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

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

TEST_F(SharedDataFixture, SymbolCompositeTypeSameAcrossTablesAndComparable) {
  // Arrange
  SymbolEncodingBimap encoding_bimap;
  encoding_bimap.find_or_emplace("a"sv);
  encoding_bimap.find_or_emplace("b"sv);

  // Act
  const SymbolDecodingTable decoding_table(encoding_bimap);
  const auto from_encoding = encoding_bimap[0];
  const auto from_decoding = decoding_table[0];

  // Assert
  EXPECT_EQ(from_encoding, from_decoding);
}

TEST_F(SharedDataFixture, LabelNameSetCompositeTypeSameAcrossTablesAndComparable) {
  // Arrange
  LabelNameSetEncodingBimap encoding_bimap;
  const LabelViewSet names{{"n1", "v1"}, {"n2", "v2"}};
  encoding_bimap.find_or_emplace(names.names());

  // Act
  const LabelNameSetDecodingTable decoding_table(encoding_bimap);
  const auto from_encoding = encoding_bimap[0];
  const auto from_decoding = decoding_table[0];

  // Assert
  EXPECT_TRUE(std::ranges::equal(from_encoding, from_decoding));
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

TEST_F(SharedDataFixture, LabelSetCompositeHashMatchesOriginalLabelSet) {
  // Arrange
  LabelSetEncodingBimap encoding_bimap;
  const LabelViewSet label_set{{"k1", "v1"}, {"k2", "v2"}};
  encoding_bimap.find_or_emplace(label_set);

  // Act
  const LabelSetDecodingTable decoding_table(encoding_bimap);
  const auto composite = decoding_table[0];
  const auto original_hash = PromPP::Primitives::hash::hash_of_label_set(label_set);
  const auto composite_hash = hash_value(composite);

  // Assert
  EXPECT_EQ(original_hash, composite_hash);
}

TEST_F(SharedDataFixture, SymbolViewIteratorStopsAtItemWithReallocatedData) {
  // Arrange
  SymbolEncodingBimap encoding_bimap;
  encoding_bimap.find_or_emplace("x"sv);
  encoding_bimap.reserve(1024);

  const SymbolDecodingTable decoding_table(encoding_bimap);

  const std::string big(1000, 'y');
  encoding_bimap.find_or_emplace(std::string_view{big});

  // Act
  std::vector<std::string_view> iterated;
  std::ranges::copy(decoding_table.data_view(), std::back_inserter(iterated));

  // Assert
  EXPECT_EQ(std::vector{"x"sv}, iterated);
}

TEST_F(SharedDataFixture, LabelNameSetViewIteratorStopsAtItemWithReallocatedData) {
  // Arrange
  LabelNameSetEncodingBimap encoding_bimap;
  const LabelViewSet baseline{{"a", "1"}};
  encoding_bimap.find_or_emplace(baseline.names());
  encoding_bimap.reserve(1024);

  const LabelNameSetDecodingTable decoding_table(encoding_bimap);

  encoding_bimap.find_or_emplace(LabelViewSet{{"very_very_very_long_label_name_which_trigger_memory_reallocation1", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation2", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation3", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation4", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation5", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation6", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation7", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation8", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation9", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation10", "1"}}
                                     .names());

  // Act
  std::vector<LabelNameSetDecodingTable::value_type> iterated;
  std::ranges::copy(decoding_table.data_view(), std::back_inserter(iterated));

  // Assert
  ASSERT_EQ(1U, iterated.size());
  EXPECT_TRUE(std::ranges::equal(baseline.names(), iterated.front()));
}

TEST_F(SharedDataFixture, LabelSetViewIteratorStopsAtItemWithReallocatedData) {
  // Arrange
  LabelSetEncodingBimap encoding_bimap;
  const LabelViewSet baseline{{"a", "1"}};
  encoding_bimap.find_or_emplace(baseline);
  encoding_bimap.reserve(1024);

  const LabelSetDecodingTable decoding_table(encoding_bimap);

  encoding_bimap.find_or_emplace(LabelViewSet{{"very_very_very_long_label_name_which_trigger_memory_reallocation1", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation2", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation3", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation4", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation5", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation6", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation7", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation8", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation9", "1"},
                                              {"very_very_very_long_label_name_which_trigger_memory_reallocation10", "1"}});

  // Act
  std::vector<LabelSetDecodingTable::value_type> iterated;
  std::ranges::copy(decoding_table.data_view(), std::back_inserter(iterated));

  // Assert
  ASSERT_EQ(1U, iterated.size());
  EXPECT_TRUE(std::ranges::equal(baseline, iterated.front()));
}

TEST_F(SharedDataFixture, IterateOverValues) {
  // Arrange
  LabelSetEncodingBimap encoding_bimap;
  encoding_bimap.find_or_emplace(LabelViewSet{{"name1", "value"}});

  // Act
  const LabelSetDecodingTable decoding_table(encoding_bimap);
  encoding_bimap.find_or_emplace(LabelViewSet{{"added1", "value"}});

  // Assert
  EXPECT_TRUE(std::ranges::equal(std::vector{"value"sv}, decoding_table.data_view().values()));
  EXPECT_EQ(1U, decoding_table.data_view().values().size());
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
