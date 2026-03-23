#include <sstream>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "primitives/snug_composites.h"

namespace {
using BareBones::Vector;
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
  encoding_table_.save(ss, checkpoint);
  decoding_table_.load(ss);

  // Assert
  EXPECT_EQ(2U, decoding_table_.size());
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
  EXPECT_EQ(2U, decoding_table_.size());
  EXPECT_EQ(symbol1, decoding_table_[id1]);
  EXPECT_EQ(symbol2, decoding_table_[id2]);
}

}  // namespace
