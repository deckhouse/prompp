#pragma once

#include <memory>
#include <variant>

#include "bare_bones/exception.h"
#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace entrypoint::head {

using LsIdsSlice = BareBones::Vector<PromPP::Primitives::LabelSetID>;
using LsIdsSlicePtr = std::unique_ptr<LsIdsSlice>;

enum class LssType : uint32_t {
  kEncodingBimap = 0,
  kQueryableEncodingBimap,
  kReadonlyLss,
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
using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament,
                                                                    SharedVectorWithChangesDetection,
                                                                    series_index::trie::CedarTrie>;

class ReadonlyLss : public PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpanWithChangesDetection> {
 public:
  using Base = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpanWithChangesDetection>;
  using SortingIndex = series_index::SortingIndex<SharedSpanWithChangesDetection>;
  using Base::Base;

  explicit ReadonlyLss(const QueryableEncodingBimap& lss) : Base(lss), sorting_index_(lss.sorting_index()) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const SortingIndex& sorting_index() const noexcept { return sorting_index_; }

 private:
  SortingIndex sorting_index_;
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

using LssVariant = std::variant<EncodingBimap, QueryableEncodingBimap, ReadonlyLss>;
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

inline LssVariantPtr create_readonly_lss(LssVariant& lss_variant) {
  switch (static_cast<LssType>(lss_variant.index())) {
    case LssType::kEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonlyLss)>, std::get<EncodingBimap>(lss_variant));
    }

    case LssType::kQueryableEncodingBimap: {
      auto& lss = std::get<QueryableEncodingBimap>(lss_variant);
      lss.build_deferred_indexes();
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonlyLss)>, lss);
    }

    default: {
      throw BareBones::Exception(0x8e6a06385b011215, "Readonly lss can't be created");
    }
  }
}

}  // namespace entrypoint::head
