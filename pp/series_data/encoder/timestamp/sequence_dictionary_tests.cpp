#include <bit>
#include <map>
#include <random>
#include <set>

#include <gtest/gtest.h>

#include "series_data/encoder/timestamp/sequence_dictionary.h"

namespace {

using series_data::encoder::timestamp::extend_hash;
using series_data::encoder::timestamp::kEmptySequenceHash;
using series_data::encoder::timestamp::kInvalidSequenceId;
using series_data::encoder::timestamp::SequenceDictionary;
using series_data::encoder::timestamp::SequenceId;
using series_data::encoder::timestamp::TimestampDecoder;
using Timestamps = BareBones::Vector<int64_t>;

[[nodiscard]] uint64_t hash_of(std::initializer_list<int64_t> timestamps) {
  uint64_t hash = kEmptySequenceHash;
  for (const auto timestamp : timestamps) {
    hash = extend_hash(hash, timestamp);
  }
  return hash;
}

// Reverses the hash mixing steps to find a timestamp extending `hash` to `target`.
[[nodiscard]] int64_t timestamp_reaching(uint64_t hash, uint64_t target) {
  constexpr uint64_t kMultiplier = 0x2545f4914f6cdd1dULL;
  uint64_t inverse = kMultiplier;
  for (int i = 0; i < 6; ++i) {
    inverse *= 2 - kMultiplier * inverse;
  }
  uint64_t x = target * inverse;
  x ^= (x >> 27) ^ (x >> 54);
  x ^= (x << 25) ^ (x << 50);
  x ^= (x >> 12) ^ (x >> 24) ^ (x >> 36) ^ (x >> 48) ^ (x >> 60);
  return static_cast<int64_t>(x ^ std::rotl(hash, 29));
}

// Finds a hash collision with strictly increasing timestamps.
[[nodiscard]] std::pair<int64_t, int64_t> colliding_with(std::initializer_list<int64_t> target) {
  int64_t start = 201;
  while (timestamp_reaching(hash_of({start}), hash_of(target)) <= start) {
    ++start;
  }
  return {start, timestamp_reaching(hash_of({start}), hash_of(target))};
}

class SequenceDictionaryFixture : public testing::Test {
 protected:
  SequenceDictionary<BareBones::DefaultReallocator> dictionary_;

  void append_interleaved(SequenceId& first_a, SequenceId& first_b, SequenceId& second_a, SequenceId& second_b, int64_t timestamp) {
    first_a = dictionary_.append(first_a, timestamp);
    first_b = dictionary_.append(first_b, timestamp + 100);
    second_a = dictionary_.append(second_a, timestamp);
    second_b = dictionary_.append(second_b, timestamp + 100);
  }

  [[nodiscard]] SequenceId append(SequenceId sequence_id, std::initializer_list<int64_t> timestamps) {
    for (const auto timestamp : timestamps) {
      sequence_id = dictionary_.append(sequence_id, timestamp);
    }
    return sequence_id;
  }

  [[nodiscard]] Timestamps decode(SequenceId sequence_id) const {
    const auto& stream = dictionary_.stream(sequence_id);
    return TimestampDecoder::decode_all(stream.reader(), stream.count());
  }

  [[nodiscard]] Timestamps decode_finalized(SequenceId stream_id) const {
    const auto& stream = dictionary_.finalized_stream(stream_id);
    return TimestampDecoder::decode_all(stream.reader(), stream.count());
  }

  struct WorkloadResult {
    uint32_t mismatches{};
    uint32_t duplicated_contents{};
  };

  // Exercises lag, divergence and restart against decoded content and unique ids.
  [[nodiscard]] WorkloadResult run_random_workload(uint32_t seed) {
    struct Series {
      SequenceId sequence_id{kInvalidSequenceId};
      Timestamps open;
    };
    struct Finalized {
      SequenceId stream_id;
      Timestamps expected;
    };

    std::mt19937 rng(seed);
    std::vector<Series> series(48);
    std::vector<Finalized> finalized;
    for (int64_t tick = 1; tick <= 1500; ++tick) {
      const int64_t clock = 1000 + 30 * tick + static_cast<int64_t>(rng() % 2);
      for (auto& s : series) {
        if (rng() % 8 == 0) {
          continue;
        }
        if (s.open.size() >= 120 || (s.open.size() >= 6 && rng() % 64 == 0)) {
          finalized.push_back({.stream_id = dictionary_.finalize(s.sequence_id), .expected = s.open});
          s = Series{};
        }
        s.sequence_id = dictionary_.append(s.sequence_id, clock);
        s.open.push_back(clock);
      }
      if (rng() % 50 == 0 && !finalized.empty()) {
        const auto index = rng() % finalized.size();
        dictionary_.release_finalized(finalized[index].stream_id);
        finalized.erase(finalized.begin() + static_cast<std::ptrdiff_t>(index));
      }
    }

    WorkloadResult result;
    std::map<std::vector<int64_t>, std::set<SequenceId>> ids_by_content;
    for (const auto& s : series) {
      if (!s.open.empty()) {
        result.mismatches += decode(s.sequence_id) != s.open;
        ids_by_content[{s.open.begin(), s.open.end()}].insert(s.sequence_id);
      }
    }
    for (const auto& f : finalized) {
      result.mismatches += decode_finalized(f.stream_id) != f.expected;
      ids_by_content[{f.expected.begin(), f.expected.end()}].insert(f.stream_id);
    }
    for (const auto& [content, ids] : ids_by_content) {
      result.duplicated_contents += ids.size() > 1;
    }

    for (const auto& s : series) {
      if (!s.open.empty()) {
        dictionary_.release(s.sequence_id);
      }
    }
    for (const auto& f : finalized) {
      dictionary_.release_finalized(f.stream_id);
    }
    return result;
  }
};

TEST_F(SequenceDictionaryFixture, AppendBuildsSequence) {
  // Arrange

  // Act
  const auto sequence_id = append(kInvalidSequenceId, {101, 102, 104});

  // Assert
  EXPECT_EQ((Timestamps{101, 102, 104}), decode(sequence_id));
  EXPECT_EQ(3U, dictionary_.count(sequence_id));
  EXPECT_EQ(104, dictionary_.last_timestamp(sequence_id));
}

TEST_F(SequenceDictionaryFixture, SequencesDifferingInLastTimestampStayApart) {
  // Arrange

  // Act
  const auto first = append(kInvalidSequenceId, {101, 102});
  const auto second = append(kInvalidSequenceId, {101, 103});

  // Assert
  EXPECT_NE(first, second);
  EXPECT_EQ((Timestamps{101, 102}), decode(first));
  EXPECT_EQ((Timestamps{101, 103}), decode(second));
}

TEST_F(SequenceDictionaryFixture, SequencesDifferingInPrefixStayApart) {
  // Arrange

  // Act
  const auto first = append(kInvalidSequenceId, {101, 102, 103});
  const auto second = append(kInvalidSequenceId, {100, 102, 103});

  // Assert
  EXPECT_NE(first, second);
  EXPECT_EQ((Timestamps{101, 102, 103}), decode(first));
  EXPECT_EQ((Timestamps{100, 102, 103}), decode(second));
}

TEST_F(SequenceDictionaryFixture, SequencesWithEqualHashCoexistAndAreFoundByContent) {
  // Arrange
  const auto [start, colliding] = colliding_with({101, 102});
  const auto first = append(kInvalidSequenceId, {101, 102});
  const auto second = append(kInvalidSequenceId, {start, colliding});

  // Act
  const auto first_again = append(kInvalidSequenceId, {101, 102});
  const auto second_again = append(kInvalidSequenceId, {start, colliding});

  // Assert
  EXPECT_EQ(hash_of({101, 102}), hash_of({start, colliding}));
  EXPECT_NE(first, second);
  EXPECT_EQ(first, first_again);
  EXPECT_EQ(second, second_again);
  EXPECT_EQ((Timestamps{101, 102}), decode(first));
  EXPECT_EQ((Timestamps{start, colliding}), decode(second));
}

TEST_F(SequenceDictionaryFixture, SequencesWithEqualHashLastTimestampAndCountStayApart) {
  // Arrange
  const auto [start, colliding] = colliding_with({101, 102});
  const auto first = append(kInvalidSequenceId, {101, 102});
  const auto second = append(kInvalidSequenceId, {start, colliding});
  const int64_t later = colliding + 1;

  // Act
  const auto first_grown = dictionary_.append(first, later);
  const auto second_grown = dictionary_.append(second, later);

  // Assert
  EXPECT_EQ(hash_of({101, 102, later}), hash_of({start, colliding, later}));
  EXPECT_NE(first_grown, second_grown);
  EXPECT_EQ((Timestamps{101, 102, later}), decode(first_grown));
  EXPECT_EQ((Timestamps{start, colliding, later}), decode(second_grown));
}

TEST_F(SequenceDictionaryFixture, SoleOwnerGrowsInPlace) {
  // Arrange
  const auto sequence_id = append(kInvalidSequenceId, {101, 102});
  const auto slots = dictionary_.slot_count();

  // Act
  const auto grown = append(sequence_id, {103, 105});

  // Assert
  EXPECT_EQ(sequence_id, grown);
  EXPECT_EQ(slots, dictionary_.slot_count());
  EXPECT_EQ((Timestamps{101, 102, 103, 105}), decode(grown));
}

TEST_F(SequenceDictionaryFixture, SharedSequenceKeepsItsContentWhenOneOwnerGrows) {
  // Arrange
  const auto lagging = append(kInvalidSequenceId, {101, 102});
  const auto leader = append(kInvalidSequenceId, {101, 102});

  // Act
  const auto grown = append(leader, {103});

  // Assert
  EXPECT_NE(lagging, grown);
  EXPECT_EQ((Timestamps{101, 102}), decode(lagging));
  EXPECT_EQ((Timestamps{101, 102, 103}), decode(grown));
}

TEST_F(SequenceDictionaryFixture, LaggingSeriesCatchesUpWithItsGroup) {
  // Arrange
  const auto lagging_start = append(kInvalidSequenceId, {101, 102});
  const auto group = append(append(kInvalidSequenceId, {101, 102}), {103, 104, 105});

  // Act
  const auto lagging = append(lagging_start, {103, 104, 105});

  // Assert
  EXPECT_EQ(group, lagging);
  EXPECT_EQ(1U, dictionary_.open_sequences_count());
}

TEST_F(SequenceDictionaryFixture, InterleavedGroupsShareTheirSequences) {
  // Arrange
  auto first_a = append(kInvalidSequenceId, {101, 102});
  auto first_b = append(kInvalidSequenceId, {201, 202});
  auto second_a = append(kInvalidSequenceId, {101, 102});
  auto second_b = append(kInvalidSequenceId, {201, 202});

  // Act
  append_interleaved(first_a, first_b, second_a, second_b, 103);
  append_interleaved(first_a, first_b, second_a, second_b, 104);
  append_interleaved(first_a, first_b, second_a, second_b, 105);

  // Assert
  EXPECT_EQ(first_a, second_a);
  EXPECT_EQ(first_b, second_b);
  EXPECT_EQ((Timestamps{101, 102, 103, 104, 105}), decode(second_a));
  EXPECT_EQ((Timestamps{201, 202, 203, 204, 205}), decode(second_b));
  EXPECT_EQ(2U, dictionary_.open_sequences_count());
}

TEST_F(SequenceDictionaryFixture, SeriesAppendedOneAfterAnotherEndOnOneSequence) {
  // Arrange
  const auto first = append(kInvalidSequenceId, {101, 102, 103, 104});

  // Act
  const auto second = append(kInvalidSequenceId, {101, 102, 103, 104});

  // Assert
  EXPECT_EQ(first, second);
  EXPECT_EQ(1U, dictionary_.open_sequences_count());
}

TEST_F(SequenceDictionaryFixture, FinalizeKeepsContentAndId) {
  // Arrange
  const auto sequence_id = append(kInvalidSequenceId, {101, 102, 104});

  // Act
  const auto stream_id = dictionary_.finalize(sequence_id);

  // Assert
  EXPECT_EQ(sequence_id, stream_id);
  EXPECT_EQ((Timestamps{101, 102, 104}), decode_finalized(stream_id));
  EXPECT_EQ(0U, dictionary_.open_sequences_count());
  EXPECT_EQ(1U, dictionary_.finalized_sequences_count());
}

TEST_F(SequenceDictionaryFixture, OpenSequenceReachingFinalizedContentSharesIt) {
  // Arrange
  const auto finalized = dictionary_.finalize(append(kInvalidSequenceId, {101, 102}));

  // Act
  const auto open = append(kInvalidSequenceId, {101, 102});

  // Assert
  EXPECT_EQ(finalized, open);
  EXPECT_EQ(1U, dictionary_.open_sequences_count());
  EXPECT_EQ(1U, dictionary_.finalized_sequences_count());
}

TEST_F(SequenceDictionaryFixture, GrowingFromTrimmedSequenceCopiesIt) {
  // Arrange
  const auto finalized = dictionary_.finalize(append(kInvalidSequenceId, {101, 102}));
  const auto open = append(kInvalidSequenceId, {101, 102});
  dictionary_.release_finalized(finalized);

  // Act
  const auto grown = append(open, {103, 104});

  // Assert
  EXPECT_EQ((Timestamps{101, 102, 103, 104}), decode(grown));
  EXPECT_EQ(1U, dictionary_.open_sequences_count());
  EXPECT_EQ(0U, dictionary_.finalized_sequences_count());
}

TEST_F(SequenceDictionaryFixture, SequenceLeftToFinalizedChunksIsTrimmedWhenLastOpenOwnerLeaves) {
  // Arrange
  const auto finalized = dictionary_.finalize(append(kInvalidSequenceId, {101, 102}));
  const auto open = append(kInvalidSequenceId, {101, 102});
  const auto finalized_again = dictionary_.finalize(append(kInvalidSequenceId, {101, 102}));

  // Act
  dictionary_.release(open);

  // Assert
  EXPECT_EQ(finalized, finalized_again);
  EXPECT_TRUE(dictionary_.finalized_stream(finalized).stream.is_read_only());
  EXPECT_EQ((Timestamps{101, 102}), decode_finalized(finalized));
  EXPECT_EQ(0U, dictionary_.open_sequences_count());
}

TEST_F(SequenceDictionaryFixture, ReleaseOfLastReferenceFreesSequence) {
  // Arrange
  const auto first = append(kInvalidSequenceId, {101, 102});
  const auto second = append(kInvalidSequenceId, {101, 102});

  // Act
  dictionary_.release(first);
  dictionary_.release(second);

  // Assert
  EXPECT_EQ(0U, dictionary_.open_sequences_count());
}

TEST_F(SequenceDictionaryFixture, RandomWorkloadReferencesEachContentOnce) {
  // Arrange

  // Act
  const auto result = run_random_workload(7);

  // Assert
  EXPECT_EQ(0U, result.mismatches);
  EXPECT_EQ(0U, result.duplicated_contents);
  EXPECT_EQ(0U, dictionary_.open_sequences_count());
  EXPECT_EQ(0U, dictionary_.finalized_sequences_count());
}

}  // namespace
