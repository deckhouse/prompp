#include <gmock/gmock.h>

#include "bare_bones/vector.h"
#include "series_index/sorting_index.h"

PRAGMA_DIAGNOSTIC(push)
PRAGMA_DIAGNOSTIC(ignored "-Warray-bounds")
#include <parallel_hashmap/btree.h>
PRAGMA_DIAGNOSTIC(pop)

namespace {

using series_index::SortingIndexBuilder;
using std::string_view_literals::operator""sv;

class SortingIndexFixture : public testing::Test {
 public:
  static constexpr std::array kItems = {"b"sv, "d"sv, "a"sv, "c"sv};

 protected:
  struct LessComparator {
    PROMPP_ALWAYS_INLINE static bool operator()(uint32_t a, uint32_t b) noexcept { return kItems[a] < kItems[b]; }
  };
  using Set = phmap::btree_set<uint32_t, LessComparator>;

  template <class T>
  using SharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;

  template <class T>
  using SharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

  Set set_{{}, LessComparator{}};
  SortingIndexBuilder<Set, BareBones::Vector, kItems.size() + 1> index_{set_};
};

constexpr uint32_t operator""_idx(const char* value, size_t len) noexcept {
  return std::ranges::find(SortingIndexFixture::kItems, std::string_view(value, len)) - SortingIndexFixture::kItems.begin();
}

TEST_F(SortingIndexFixture, BuildAndSort) {
  // Arrange
  set_.emplace(0);
  set_.emplace(1);
  set_.emplace(2);
  set_.emplace(3);
  std::array series_ids{"d"_idx, "b"_idx, "c"_idx, "a"_idx};

  // Act
  index_.build();
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, UpdateAndSort) {
  // Arrange
  set_.emplace(0);
  index_.build();

  std::array series_ids{"d"_idx, "b"_idx, "a"_idx};

  // Act
  index_.update(set_.emplace(1).first);
  index_.update(set_.emplace(2).first);
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, ResetIndexOnUpdateError) {
  // Arrange
  set_.emplace(0);
  index_.build();

  // Act
  index_.update(set_.emplace(1).first);
  index_.update(set_.emplace(2).first);
  index_.update(set_.emplace(3).first);

  // Assert
  EXPECT_TRUE(index_.empty());
}

TEST_F(SortingIndexFixture, BuildIndexInSort) {
  // Arrange
  set_.emplace(0);
  set_.emplace(1);
  set_.emplace(2);
  set_.emplace(3);
  std::array series_ids{"d"_idx, "b"_idx, "c"_idx, "a"_idx};

  // Act
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, BuildIndexOutOfOrderInSort) {
  // Arrange
  set_.emplace(2);
  set_.emplace(3);
  set_.emplace(0);
  set_.emplace(1);

  std::array series_ids{"d"_idx, "c"_idx, "b"_idx, "a"_idx};

  // Act
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

TEST_F(SortingIndexFixture, BuildIndexOutOfOrderWithSkipInSort) {
  // Arrange
  set_.emplace(3);
  set_.emplace(0);

  std::array series_ids{"c"_idx, "b"_idx};

  // Act
  index_.sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_FALSE(index_.empty());
  EXPECT_THAT(series_ids, testing::ElementsAre("b"_idx, "c"_idx));
}

TEST_F(SortingIndexFixture, IndexSnapshot) {
  // Arrange
  SortingIndexBuilder<Set, SharedVector, kItems.size() + 1> index{set_};
  set_.emplace(0);
  set_.emplace(1);
  set_.emplace(2);
  set_.emplace(3);
  std::array series_ids{"d"_idx, "b"_idx, "c"_idx, "a"_idx};

  // Act
  index.build();
  series_index::SortingIndex<SharedSpan>(index.index()).sort(series_ids.begin(), series_ids.end());

  // Assert
  EXPECT_THAT(series_ids, testing::ElementsAre("a"_idx, "b"_idx, "c"_idx, "d"_idx));
}

}  // namespace