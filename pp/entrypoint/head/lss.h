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
  kReadonly,
};

using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::SharedVector>;
using OrderedEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap<BareBones::Vector>;
using ReadonlyLss = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::SharedSpan>;

template <class T>
using QueryableEncodingBimapVector = BareBones::SharedVector<T>;
using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, QueryableEncodingBimapVector, TrieIndex>;

template <class Lss>
concept readonly_lss_constructible_from = std::is_same_v<Lss, QueryableEncodingBimap> || std::is_same_v<Lss, EncodingBimap>;

using LssVariant = std::variant<EncodingBimap, OrderedEncodingBimap, QueryableEncodingBimap, ReadonlyLss>;
using LssVariantPtr = std::unique_ptr<LssVariant>;

using ReadonlyLssPtr = std::unique_ptr<ReadonlyLss>;

static_assert(sizeof(LssVariantPtr) == sizeof(void*));
static_assert(sizeof(ReadonlyLssPtr) == sizeof(void*));

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
  return std::visit(
      []<class Lss>(const Lss& lss) -> LssVariantPtr {
        if constexpr (readonly_lss_constructible_from<Lss>) {
          return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonly)>, lss);
        }

        throw BareBones::Exception(0x8e6a06385b011215, "Readonly lss can't be created");
      },
      lss_variant);
}

template <class Lss>
inline LssVariantPtr create_readonly_lss(const Lss& lss) {
  if constexpr (readonly_lss_constructible_from<Lss>) {
    return std::make_unique<LssVariant>(std::in_place_index<static_cast<int>(LssType::kReadonly)>, lss);
  }

  throw BareBones::Exception(0x8e6a06385b011216, "Readonly lss can't be created");
}

}  // namespace entrypoint::head
