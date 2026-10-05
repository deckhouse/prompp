#pragma once

#include <bit>
#include <cassert>
#include <cstddef>
#include <cstring>
#include <limits>
#include <utility>

#include "parallel_hashmap/phmap.h"

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector_with_holes.h"
#include "encoder.h"
#include "metrics/metric.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::encoder::timestamp {

inline constexpr uint64_t kEmptySequenceHash = 0x9e3779b97f4a7c15ULL;

// For a fixed prefix, each mixing step is reversible, so distinct appended timestamps produce distinct hashes.
[[nodiscard]] PROMPP_ALWAYS_INLINE constexpr uint64_t extend_hash(uint64_t hash, int64_t timestamp) noexcept {
  uint64_t x = std::rotl(hash, 29) ^ static_cast<uint64_t>(timestamp);
  x ^= x >> 12;
  x ^= x << 25;
  x ^= x >> 27;
  return x * 0x2545f4914f6cdd1dULL;
}

// Owns timestamp sequences, sharing equal content under one id even when hashes collide.
// Ids are reused after the last open or finalized reference is released.
// Not thread-safe; mutations may invalidate stream references.
template <BareBones::ReallocatorInterface Reallocator>
class SequenceDictionary {
 public:
  using BitSequenceWithItemsCount = encoder::BitSequenceWithItemsCount<Reallocator>;

  SequenceDictionary() = default;
  SequenceDictionary(const SequenceDictionary&) = delete;
  SequenceDictionary(SequenceDictionary&&) = delete;
  SequenceDictionary& operator=(const SequenceDictionary&) = delete;
  SequenceDictionary& operator=(SequenceDictionary&&) = delete;

  // Consumes one open reference to `sequence_id` and returns one for the extended sequence.
  // `kInvalidSequenceId` starts an empty sequence; the one-byte count limits sequences to 255 timestamps.
  [[nodiscard]] SequenceId append(SequenceId sequence_id, int64_t timestamp) {
    if (sequence_id != kInvalidSequenceId) {
      assert(timestamp > last_timestamp(sequence_id));
      assert(count(sequence_id) < std::numeric_limits<uint8_t>::max());
    }

    if (const auto cached_id = append_cache_.result_id;
        sequence_id == append_cache_.parent_id && timestamp == append_cache_.timestamp && cached_id != kInvalidSequenceId) [[likely]] {
      return reuse_sequence(sequence_id, timestamp, cached_id);
    }

    if (sequence_id == kInvalidSequenceId) [[unlikely]] {
      return append_to_empty(timestamp);
    }

    const auto& parent = sequences_[sequence_id];
    const Extension extension{.hash = extend_hash(parent.hash, timestamp), .parent = &parent, .timestamp = timestamp};
    // Remove the old entry before changing content so the index remains consistent with the sequence hash.
    const bool in_place = parent.open_references == 1 && parent.finalized_references == 0 && !parent.stream.stream.is_read_only();
    if (in_place) {
      index_.erase(sequence_id);
    }

    // Creating a sequence may relocate `parent`; do not use it after insertion.
    SequenceId created_id = kInvalidSequenceId;
    const auto it = index_.lazy_emplace(extension, [&](const auto& construct) PROMPP_LAMBDA_INLINE {
      created_id = in_place ? sequence_id : copy_and_append(sequence_id, extension.hash, timestamp);
      construct(created_id);
    });
    if (created_id == kInvalidSequenceId) {
      return reuse_sequence(sequence_id, timestamp, *it);
    }
    if (in_place) {
      return append_in_place(sequence_id, extension.hash, timestamp);
    }
    cache_append(sequence_id, timestamp, created_id);
    release(sequence_id);
    return created_id;
  }

  // Converts one open reference into a finalized reference without changing the id or content.
  [[nodiscard]] SequenceId finalize(SequenceId sequence_id) noexcept {
    auto& sequence = sequences_[sequence_id];
    assert(sequence.open_references > 0);
    if (sequence.finalized_references++ == 0) {
      ++finalized_sequences_count_;
    }
    if (--sequence.open_references == 0) {
      --open_sequences_count_;
      freeze_stream(sequence);
    }
    return sequence_id;
  }

  // Releases one open reference to `sequence_id`; `kInvalidSequenceId` has no reference to release.
  PROMPP_ALWAYS_INLINE void release(SequenceId sequence_id) noexcept {
    if (sequence_id == kInvalidSequenceId) [[unlikely]] {
      return;
    }
    auto& sequence = sequences_[sequence_id];
    assert(sequence.open_references > 0);
    if (--sequence.open_references == 0) {
      --open_sequences_count_;
      if (sequence.finalized_references == 0) {
        erase(sequence_id);
      } else {
        freeze_stream(sequence);
      }
    }
  }

  // Releases one finalized reference to `sequence_id`. The sequence is freed when neither kind of reference remains.
  PROMPP_ALWAYS_INLINE void release_finalized(SequenceId sequence_id) noexcept {
    auto& sequence = sequences_[sequence_id];
    assert(sequence.finalized_references > 0);
    if (--sequence.finalized_references == 0) {
      --finalized_sequences_count_;
      if (sequence.open_references == 0) {
        erase(sequence_id);
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BitSequenceWithItemsCount& stream(SequenceId sequence_id) const noexcept { return sequences_[sequence_id].stream; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t last_timestamp(SequenceId sequence_id) const noexcept { return sequences_[sequence_id].encoder.timestamp(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint8_t count(SequenceId sequence_id) const noexcept { return sequences_[sequence_id].count; }

  // Includes reusable holes left by erased sequences.
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t slot_count() const noexcept { return sequences_.size(); }
  // Sequences with both kinds of reference contribute to both counts.
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t open_sequences_count() const noexcept { return open_sequences_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t finalized_sequences_count() const noexcept { return finalized_sequences_count_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return sequences_.allocated_memory() + index_allocated_memory_; }

  // A non-null `slot_count_gauge` must outlive the dictionary.
  PROMPP_ALWAYS_INLINE void set_slot_count_gauge(metrics::Gauge* slot_count_gauge) noexcept {
    slot_count_gauge_ = slot_count_gauge;
    if (slot_count_gauge_ != nullptr) [[likely]] {
      slot_count_gauge_->set(sequences_.size());
    }
  }

 private:
  static constexpr uint64_t kNoVersion = 0;

  struct Sequence {
    uint64_t hash;
    TimestampEncoderCodec encoder;
    uint64_t version;         // Unique; changes with every change of content.
    uint64_t parent_version;  // Version of the prefix copied into this sequence
    BitSequenceWithItemsCount stream;
    uint8_t count;
    uint32_t open_references;
    uint32_t finalized_references;

    Sequence() = default;
    explicit Sequence(DoNotInitializeTag tag) noexcept : stream(tag) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream.allocated_memory(); }
  };
  static_assert(sizeof(Sequence) == 64);
  // Keep the stream pointer aligned so leak detection can follow it.
  static_assert(offsetof(Sequence, stream) % alignof(void*) == 0);

  // Lookup key for a sequence not yet encoded: a prefix followed by one timestamp.
  struct Extension {
    uint64_t hash;
    const Sequence* parent;
    int64_t timestamp;
  };

  class SequenceHash {
   public:
    using is_transparent = void;

    explicit SequenceHash(const BareBones::VectorWithHoles<Sequence, Reallocator>& sequences) noexcept : sequences_(&sequences) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(SequenceId id) const noexcept { return (*sequences_)[id].hash; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const Extension& extension) const noexcept { return extension.hash; }

   private:
    const BareBones::VectorWithHoles<Sequence, Reallocator>* sequences_;
  };

  class SequenceEqual {
   public:
    using is_transparent = void;

    explicit SequenceEqual(const BareBones::VectorWithHoles<Sequence, Reallocator>& sequences) noexcept : sequences_(&sequences) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator()(SequenceId a, SequenceId b) const noexcept { return a == b; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator()(SequenceId id, const Extension& extension) const noexcept {
      return matches_extension((*sequences_)[id], extension);
    }

   private:
    const BareBones::VectorWithHoles<Sequence, Reallocator>* sequences_;
  };

  BareBones::VectorWithHoles<Sequence, Reallocator> sequences_;
  size_t index_allocated_memory_{};
  phmap::flat_hash_set<SequenceId, SequenceHash, SequenceEqual, BareBones::Allocator<SequenceId, Reallocator>> index_{
      0, SequenceHash{sequences_}, SequenceEqual{sequences_}, BareBones::Allocator<SequenceId, Reallocator>{index_allocated_memory_}};

  // Repeating a parent and timestamp reuses the result
  struct AppendCache {
    SequenceId parent_id{kInvalidSequenceId};
    SequenceId result_id{kInvalidSequenceId};
    int64_t timestamp{std::numeric_limits<int64_t>::min()};
  } append_cache_;

  uint64_t next_version_{kNoVersion + 1};
  uint32_t open_sequences_count_{};
  uint32_t finalized_sequences_count_{};
  metrics::Gauge* slot_count_gauge_{};

  // Each timestamp code ends at an unambiguous bit boundary: equal prefix bits, count and last timestamp prove equal content.
  // A matching parent version identifies the same unchanged prefix without reading its bits.
  [[nodiscard]] static bool matches_extension(const Sequence& candidate, const Extension& extension) noexcept {
    if (candidate.hash != extension.hash || candidate.encoder.timestamp() != extension.timestamp) {
      return false;
    }
    if (extension.parent == nullptr) {
      return candidate.count == 1;
    }
    const auto& parent = *extension.parent;
    if (candidate.count != parent.count + 1) {
      return false;
    }
    return candidate.parent_version == parent.version || has_timestamp_prefix(candidate.stream.stream, parent.stream.stream);
  }

  // The first byte stores the count, not timestamp content, and must be excluded from the prefix comparison.
  [[nodiscard]] static bool has_timestamp_prefix(const CompactBitSequence<Reallocator>& sequence, const CompactBitSequence<Reallocator>& prefix) noexcept {
    const auto prefix_bits = prefix.size_in_bits();
    if (sequence.size_in_bits() < prefix_bits) {
      return false;
    }
    constexpr uint32_t kCountBytes = 1;
    const auto full_bytes = prefix_bits / BareBones::Bit::kByteBits;
    if (std::memcmp(sequence.raw_bytes() + kCountBytes, prefix.raw_bytes() + kCountBytes, full_bytes - kCountBytes) != 0) {
      return false;
    }
    const auto tail_bits = prefix_bits % BareBones::Bit::kByteBits;
    const auto mask = static_cast<uint8_t>((1U << tail_bits) - 1);
    return tail_bits == 0 || (sequence.raw_bytes()[full_bytes] & mask) == (prefix.raw_bytes()[full_bytes] & mask);
  }

  // Shrinking makes the buffer read-only. Freeze only once; subsequent growth must copy it.
  static void freeze_stream(Sequence& sequence) noexcept {
    if (!sequence.stream.stream.is_read_only()) {
      sequence.stream.stream.shrink_to_fit();
    }
  }

  SequenceId reuse_sequence(SequenceId sequence_id, int64_t timestamp, SequenceId result_id) noexcept {
    if (sequences_[result_id].open_references++ == 0) {
      ++open_sequences_count_;
    }
    cache_append(sequence_id, timestamp, result_id);
    release(sequence_id);
    return result_id;
  }

  SequenceId append_to_empty(int64_t timestamp) {
    const Extension extension{.hash = extend_hash(kEmptySequenceHash, timestamp), .parent = nullptr, .timestamp = timestamp};
    SequenceId created_id = kInvalidSequenceId;
    const auto it = index_.lazy_emplace(extension, [&](const auto& construct) PROMPP_LAMBDA_INLINE {
      auto& sequence = emplace_sequence();
      TimestampEncoder::encode_first(sequence.encoder, timestamp, sequence.stream);
      sequence.count = 1;
      sequence.hash = extension.hash;
      sequence.version = next_version_++;
      sequence.parent_version = kNoVersion;
      created_id = sequences_.index_of(sequence);
      construct(created_id);
    });
    if (created_id == kInvalidSequenceId) {
      return reuse_sequence(kInvalidSequenceId, timestamp, *it);
    }
    cache_append(kInvalidSequenceId, timestamp, created_id);
    return created_id;
  }

  // The index already holds `sequence_id` under `hash`; update its content before another index operation.
  SequenceId append_in_place(SequenceId sequence_id, uint64_t hash, int64_t timestamp) {
    auto& sequence = sequences_[sequence_id];
    TimestampEncoder::encode(sequence.encoder, timestamp, sequence.stream);
    ++sequence.count;
    sequence.hash = hash;
    sequence.version = next_version_++;
    sequence.parent_version = kNoVersion;
    if (sequence_id == append_cache_.parent_id || sequence_id == append_cache_.result_id) {
      append_cache_ = {};
    }
    return sequence_id;
  }

  // Copy into a writable buffer because the parent may have been shrunk to read-only storage.
  SequenceId copy_and_append(SequenceId parent_id, uint64_t hash, int64_t timestamp) {
    auto& sequence = emplace_sequence(DoNotInitializeTag{});
    const auto& parent = sequences_[parent_id];
    sequence.stream.stream.push_back_bytes(parent.stream.stream.raw_bytes(), parent.stream.stream.size_in_bits());
    sequence.encoder = parent.encoder;
    sequence.count = parent.count + 1;
    sequence.hash = hash;
    sequence.version = next_version_++;
    sequence.parent_version = parent.version;
    TimestampEncoder::encode(sequence.encoder, timestamp, sequence.stream);
    return sequences_.index_of(sequence);
  }

  template <class... Args>
  Sequence& emplace_sequence(Args&&... args) {
    auto& sequence = sequences_.emplace_back(std::forward<Args>(args)...);
    sequence.open_references = 1;
    sequence.finalized_references = 0;
    ++open_sequences_count_;
    if (slot_count_gauge_ != nullptr) [[likely]] {
      slot_count_gauge_->set(sequences_.size());
    }
    return sequence;
  }

  void erase(SequenceId sequence_id) noexcept {
    index_.erase(sequence_id);
    if (sequence_id == append_cache_.parent_id || sequence_id == append_cache_.result_id) {
      append_cache_ = {};
    }
    sequences_.erase(sequence_id);
  }

  PROMPP_ALWAYS_INLINE void cache_append(SequenceId parent_id, int64_t timestamp, SequenceId result_id) noexcept {
    append_cache_ = {.parent_id = parent_id, .result_id = result_id, .timestamp = timestamp};
  }
};

}  // namespace series_data::encoder::timestamp
