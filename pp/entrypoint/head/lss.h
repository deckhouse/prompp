#pragma once

#include <variant>

#include "bare_bones/exception.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace entrypoint::head {

enum class LssType : uint32_t {
  kEncodingBimap = 0,
  kOrderedEncodingBimap,
  kQueryableEncodingBimap,
  kReadonlyEncodingBimap,
  kReadonlyQueryableEncodingBimap,
};

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using OrderedEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap<BareBones::Vector>;

namespace lss_memory {

static thread_local bool has_reallocations{};

struct Reallocator {
  PROMPP_ALWAYS_INLINE static void* reallocate(void* memory, size_t size) {
    const auto result = std::realloc(memory, size);
    if (result != memory) {
      has_reallocations = true;
    }
    return result;
  }
  PROMPP_ALWAYS_INLINE static void free(void* memory) { return std::free(memory); }
};

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

using ReadonlyQueryableEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpanWithChangesDetection>;
using ReadonlyEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpan>;

using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<SharedVector>;
using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, SharedVectorWithChangesDetection, TrieIndex>;

using LssVariant = std::variant<EncodingBimap, OrderedEncodingBimap, QueryableEncodingBimap, ReadonlyEncodingBimap, ReadonlyQueryableEncodingBimap>;
using LssVariantPtr = std::unique_ptr<LssVariant>;

static_assert(sizeof(LssVariantPtr) == sizeof(void*));

inline LssVariantPtr create_lss(LssType type) {
  switch (type) {
    case LssType::kEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kEncodingBimap)>);
    }

    case LssType::kOrderedEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kOrderedEncodingBimap)>);
    }

    case LssType::kQueryableEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kQueryableEncodingBimap)>);
    }

    default: {
      throw BareBones::Exception(0x73818a05bbeb0df1, "Invalid lss type");
    }
  }
}

inline LssVariantPtr create_readonly_lss(const LssVariant& lss_variant) {
  switch (static_cast<LssType>(lss_variant.index())) {
    case LssType::kEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonlyEncodingBimap)>, std::get<EncodingBimap>(lss_variant));
    }

    case LssType::kQueryableEncodingBimap: {
      return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonlyQueryableEncodingBimap)>,
                                          std::get<QueryableEncodingBimap>(lss_variant));
    }

    default: {
      throw BareBones::Exception(0x8e6a06385b011215, "Readonly lss can't be created");
    }
  }
}

}  // namespace entrypoint::head
