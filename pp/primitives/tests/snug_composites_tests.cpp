#include <sstream>

#include "gtest/gtest.h"

#include "bare_bones/streams.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"

namespace {
using BareBones::Vector;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;

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
  EXPECT_EQ(0U, encoding_table_.size());
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

  EXPECT_EQ(2U, encoding_table_.size());
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
  EXPECT_EQ(2U, encoding_table_.size());
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
  snapshot_stream << checkpoint1;

  encoding_table_.find_or_emplace(ls_[2]);
  encoding_table_.find_or_emplace(ls_[3]);
  const auto checkpoint2 = encoding_table_.checkpoint();
  delta1_stream << (checkpoint2 - checkpoint1);

  encoding_table_.shrink_to_checkpoint_size(checkpoint2);
  const auto checkpoint_after_shrink = encoding_table_.checkpoint();

  encoding_table_.find_or_emplace(ls_[4]);
  encoding_table_.find_or_emplace(ls_[5]);
  const auto checkpoint3 = encoding_table_.checkpoint();
  delta2_stream << (checkpoint3 - checkpoint_after_shrink);

  // Act
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> loaded_table;
  snapshot_stream >> loaded_table;
  delta1_stream >> loaded_table;
  delta2_stream >> loaded_table;

  // Assert
  EXPECT_EQ(6U, loaded_table.size());
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
  snapshot_stream << checkpoint1;

  encoding_table_.find_or_emplace(ls_[2]);
  encoding_table_.find_or_emplace(ls_[3]);
  const auto checkpoint2 = encoding_table_.checkpoint();
  delta1_stream << (checkpoint2 - checkpoint1);

  encoding_table_.shrink_to_checkpoint_size(checkpoint1);
  const auto checkpoint_after_shrink = encoding_table_.checkpoint();

  encoding_table_.find_or_emplace(ls_[4]);
  encoding_table_.find_or_emplace(ls_[5]);
  const auto checkpoint3 = encoding_table_.checkpoint();
  delta2_stream << (checkpoint3 - checkpoint_after_shrink);

  // Act
  PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<Vector> loaded_table;
  snapshot_stream >> loaded_table;
  delta1_stream >> loaded_table;
  delta2_stream >> loaded_table;

  // Assert
  EXPECT_EQ(6U, loaded_table.size());
  EXPECT_TRUE(std::ranges::equal(ls_, loaded_table, [](const auto& a, const auto& b) { return a == b; }));
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
