#pragma once

#include <cassert>
#include <memory>
#include <variant>

#include "bare_bones/exception.h"
#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"

namespace entrypoint::head {

using LsIdsSlice = BareBones::Vector<PromPP::Primitives::LabelSetID>;
using LsIdsSlicePtr = std::unique_ptr<LsIdsSlice>;

enum class LssType : uint8_t {
  kEncodingBimap = 0,
  kQueryableEncodingBimap,
};

enum class SnapshotLSSType : uint8_t {
  kSnapshotLSS = 0,
  kShrinkAwareSnapshotLSS,
};

namespace lss_memory {

thread_local inline bool has_reallocations{};

struct Reallocator {
  PROMPP_ALWAYS_INLINE static size_t allocation_size(size_t needed_size) noexcept { return BareBones::DefaultReallocator::allocation_size(needed_size); }

  PROMPP_ALWAYS_INLINE static void* reallocate(void* memory, size_t size) {
    const auto result = BareBones::DefaultReallocator::reallocate(memory, size);
    if (result != memory) [[likely]] {
      has_reallocations = true;
    }
    return result;
  }

  PROMPP_ALWAYS_INLINE static void free(void* memory) { return BareBones::DefaultReallocator::free(memory); }
};

static_assert(BareBones::ReallocatorInterface<Reallocator>);

}  // namespace lss_memory

template <class T>
using SharedMemoryWithChangesDetection = BareBones::SharedMemory<T, lss_memory::Reallocator>;

template <class T>
using SharedSpanWithChangesDetection = BareBones::SharedSpan<T, lss_memory::Reallocator>;

template <class T>
using SharedVectorWithChangesDetection = BareBones::SharedVector<T, lss_memory::Reallocator>;

template <class T>
using SharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

template <class T>
using SharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;

using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<SharedVectorWithChangesDetection>;
using QueryableEncodingBimap = series_index::QueryableEncodingBimap<SharedVectorWithChangesDetection>;

class SnapshotLSS : public PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpanWithChangesDetection> {
 public:
  using Base = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpanWithChangesDetection>;
  using SortingIndex = series_index::SortingIndex<SharedSpanWithChangesDetection>;
  using value_type = typename Base::value_type;
  using Base::Base;

  explicit SnapshotLSS(const QueryableEncodingBimap& lss) : Base(lss), sorting_index_(lss.sorting_index()) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SortingIndex& sorting_index() const noexcept { return sorting_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator[](uint32_t id) const noexcept { return Base::operator[](id); }

 private:
  SortingIndex sorting_index_;
};

class ShrinkAwareSnapshotLSS : public SnapshotLSS {
 public:
  explicit ShrinkAwareSnapshotLSS(const QueryableEncodingBimap& lss)
      : SnapshotLSS(lss), shrink_state_(lss.shrink_state().clone_for_snapshot()), added_series_(lss.added_series()) {}

  [[nodiscard]] value_type operator[](uint32_t id) const noexcept {
    if (shrink_state_.is_shrunk()) {
      return resolve_shrunk_series(id);
    }

    if (shrink_state_.is_fixed() && is_hidden_in_fixed_state(id)) {
      return value_type{};
    }

    return SnapshotLSS::operator[](id);
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_hidden_in_fixed_state(uint32_t id) const noexcept {
    return shrink_state_.is_hidden_in_fixed_state(id, added_series_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE value_type resolve_shrunk_series(uint32_t id) const noexcept {
    return shrink_state_.template resolve_shrunk_series<value_type>(
        id, [this](uint32_t storage_id) { return SnapshotLSS::operator[](storage_id); }, Base::kInvalidId);
  }

  QueryableEncodingBimap::ShrinkState shrink_state_{};
  BareBones::Bitset added_series_{};
};

template <class Lss>
class ReallocationsDetector {
 public:
  explicit ReallocationsDetector(const Lss& lss) : lss_(lss), sorting_index_buffer_(get_sorting_index_buffer()) { lss_memory::has_reallocations = false; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_reallocations() const noexcept {
    return lss_memory::has_reallocations || sorting_index_buffer_ != get_sorting_index_buffer();
  }

 private:
  const Lss& lss_;
  const uint32_t* sorting_index_buffer_{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE const uint32_t* get_sorting_index_buffer() const noexcept {
    if constexpr (std::is_same_v<Lss, QueryableEncodingBimap>) {
      return lss_.sorting_index().index.data();
    }

    return nullptr;
  }
};

using LssVariant = std::variant<EncodingBimap, QueryableEncodingBimap>;
using LssVariantPtr = std::unique_ptr<LssVariant>;

static_assert(sizeof(LssVariantPtr) == sizeof(void*));

inline LssVariantPtr create_lss(LssType type) {
  switch (type) {
    case LssType::kEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kEncodingBimap)>);
    }

    case LssType::kQueryableEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kQueryableEncodingBimap)>);
    }

    default: {
      throw BareBones::Exception(0x73818a05bbeb0df1, "Invalid lss type");
    }
  }
}

using SnapshotLSSVariant = std::variant<SnapshotLSS, ShrinkAwareSnapshotLSS>;
using SnapshotLSSVariantPtr = std::unique_ptr<SnapshotLSSVariant>;

static_assert(sizeof(SnapshotLSSVariantPtr) == sizeof(void*));

inline SnapshotLSSVariantPtr create_snapshot_lss(LssVariant& lss_variant) {
  switch (static_cast<LssType>(lss_variant.index())) {
    case LssType::kEncodingBimap: {
      return std::make_unique<SnapshotLSSVariant>(std::in_place_index<static_cast<int>(SnapshotLSSType::kSnapshotLSS)>, std::get<EncodingBimap>(lss_variant));
    }

    case LssType::kQueryableEncodingBimap: {
      auto& lss = std::get<QueryableEncodingBimap>(lss_variant);
      lss.build_deferred_indexes();
      if (!lss.shrink_state().is_normal()) {
        return std::make_unique<SnapshotLSSVariant>(std::in_place_index<static_cast<int>(SnapshotLSSType::kShrinkAwareSnapshotLSS)>, lss);
      }
      return std::make_unique<SnapshotLSSVariant>(std::in_place_index<static_cast<int>(SnapshotLSSType::kSnapshotLSS)>, lss);
    }

    default: {
      throw BareBones::Exception(0x8e6a06385b011215, "Snapshot lss can't be created");
    }
  }
}

}  // namespace entrypoint::head
