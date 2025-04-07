#pragma once

#include <variant>

#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "shared_lss.h"

namespace entrypoint::head {

enum class LssType : uint32_t {
  kEncodingBimap = 0,
  kOrderedEncodingBimap,
  kQueryableEncodingBimap,
  kShared,
};

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>;
using OrderedEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap<BareBones::Vector>;

template <class T>
using QueryableEncodingBimapVector = BareBones::SharedVector<T>;
using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, QueryableEncodingBimapVector, TrieIndex>;

using LssVariant = std::variant<EncodingBimap, OrderedEncodingBimap, QueryableEncodingBimap, SharedLss>;
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
      assert(type == LssType::kEncodingBimap);
      return {};
    }
  }
}

inline LssVariantPtr create_shared_lss(const QueryableEncodingBimap& lss) {
  return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kShared)>, lss);
}

}  // namespace entrypoint::head
