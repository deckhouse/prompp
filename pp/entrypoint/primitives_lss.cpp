#include "primitives_lss.h"

#include <limits>

#include "bare_bones/bitset.h"
#include "bare_bones/vector.h"
#include "head/lss.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"
#include "series_index/querier/label_names_querier.h"
#include "series_index/querier/label_values_querier.h"
#include "series_index/querier/series_operations.h"
#include "series_index/queryable_encoding_bimap.h"

using GoLabelMatchers = PromPP::Primitives::Go::SliceView<PromPP::Prometheus::LabelMatcherTrait<PromPP::Primitives::Go::String>>;
using GoSliceOfString = PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::String>;
using GoSliceViewString = PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::String>;
using entrypoint::head::LsIdsSlice;
using entrypoint::head::LsIdsSlicePtr;
using entrypoint::head::LssType;
using entrypoint::head::LssVariantPtr;
using entrypoint::head::QueryableEncodingBimap;
using entrypoint::head::SnapshotLSSVariantPtr;

extern "C" void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    LssType lss_type;
  };
  struct Result {
    LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(static_cast<Arguments*>(args)->lss_type)};
}

extern "C" void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    LssVariantPtr lss;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    uint64_t allocated_memory;
  };

  std::visit([res](const auto& lss) { new (res) Result{.allocated_memory = lss.allocated_memory()}; }, *static_cast<Arguments*>(args)->lss);
}

struct FindOrEmplaceResult {
  uint32_t ls_id;
  bool lss_has_reallocations;
};

template <class Lss>
PROMPP_ALWAYS_INLINE FindOrEmplaceResult find_or_emplace(auto& lss, const auto& label_set) {
  if constexpr (Lss::kIsReadOnly) {
    throw BareBones::Exception(0x1b877a0ab46a69a6, "lss is readonly");
  } else {
    const entrypoint::head::ReallocationsDetector reallocation_detector(lss);
    const auto ls_id = lss.find_or_emplace(label_set);
    return {.ls_id = ls_id, .lss_has_reallocations = reallocation_detector.has_reallocations()};
  }
}

extern "C" void prompp_primitives_lss_find_or_emplace(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };

  auto in = static_cast<Arguments*>(args);
  new (res) FindOrEmplaceResult(std::visit([in]<typename Lss>(Lss& lss) { return find_or_emplace<Lss>(lss, in->label_set); }, *in->lss));
}

extern "C" void prompp_primitives_lss_find_or_emplace_builder(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr lss;
    struct {
      SnapshotLSSVariantPtr snapshot;
      uint32_t ls_id;
      SliceView<PromPP::Primitives::Go::Label> sorted_add;
      SliceView<PromPP::Primitives::Go::String> sorted_del;
    } builder;
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) FindOrEmplaceResult(std::visit(
      [&builder = in->builder]<typename Lss>(Lss& lss) {
        static const entrypoint::head::SnapshotLSS::value_type empty_label_set;
        const auto& label_set = builder.snapshot ? std::get<entrypoint::head::SnapshotLSS>(*builder.snapshot)[builder.ls_id] : empty_label_set;

        return find_or_emplace<Lss>(lss, LabelSetBuilder{label_set, builder.sorted_add, builder.sorted_del});
      },
      *in->lss));
}

struct LssQueryResult {
  PromPP::Primitives::Go::Slice<uint32_t> matches;
  PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths;
  uint32_t status;
};

using Querier = series_index::querier::Querier<PromPP::Primitives::Go::Slice>;
using SelectorPtr = std::unique_ptr<Querier::Selector>;

extern "C" void prompp_primitives_lss_query_selector(void* args, void* res) {
  using series_index::querier::QuerierStatus;
  using MatchResolver = series_index::querier::MatchResolver;
  using SelectorQuerier = series_index::querier::SelectorQuerier<QueryableEncodingBimap::TrieIndex, Querier::Selector, series_index::querier::MatchResolver>;

  struct Arguments {
    LssVariantPtr lss;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    SelectorPtr selector{std::make_unique<Querier::Selector>()};
    uint32_t status;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto& lss = std::get<QueryableEncodingBimap>(*in->lss);

  const auto out = new (res) Result();
  if (out->status = static_cast<uint32_t>(SelectorQuerier{lss.trie_index(), MatchResolver(lss.reverse_index())}.query(in->label_matchers, *out->selector));
      out->status != static_cast<uint32_t>(QuerierStatus::kMatch)) {
    out->selector.reset();
  }
}

extern "C" void prompp_primitives_snapshot_query(void* args, void* res) {
  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
    SelectorPtr selector;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<uint32_t> matches;
    PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths{};
    uint32_t status;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& snapshot_variant = *in->snapshot;
  auto query_result = Querier{}.query(*in->selector);
  in->selector.reset();
  std::visit([&query_result](const auto& snapshot) { snapshot.sorting_index().sort(query_result.series_ids); }, snapshot_variant);

  const auto out = new (res) Result{
      .matches = std::move(query_result.series_ids),
      .status = static_cast<uint32_t>(query_result.status),
  };
  out->label_set_lengths.reserve(out->matches.size());
  std::visit(
      [&out](const auto& snapshot) {
        std::ranges::transform(out->matches, std::back_inserter(out->label_set_lengths),
                               [&snapshot](const auto ls_id) PROMPP_LAMBDA_INLINE { return static_cast<uint16_t>(snapshot[ls_id].size()); });
      },
      snapshot_variant);
}

struct GroupSeriesByLabelNamesResult {
  PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::Slice<uint32_t>> groups;
};

extern "C" void prompp_primitives_group_series_by_label_names(void* args, void* res) {
  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
    PromPP::Primitives::Go::SliceView<uint32_t> label_name_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) GroupSeriesByLabelNamesResult();

  series_index::querier::group_series_by_label_names<entrypoint::head::SnapshotLSS, PromPP::Primitives::Go::Slice>(
      std::get<entrypoint::head::SnapshotLSS>(*in->snapshot), in->series_ids.span(), in->label_name_ids.span(), out->groups);
}

extern "C" void prompp_primitives_group_series_by_label_names_result_free(void* args) {
  static_cast<GroupSeriesByLabelNamesResult*>(args)->~GroupSeriesByLabelNamesResult();
}

extern "C" void prompp_primitives_lss_query_result_free(void* args) {
  static_cast<LssQueryResult*>(args)->~LssQueryResult();
}

void prompp_primitives_lss_get_label_sets(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    Slice<uint32_t> series_ids;
  };
  struct Result {
    Slice<Slice<Label>> label_sets;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        out->label_sets.resize(in->series_ids.size());

        for (size_t i = 0; i < in->series_ids.size(); ++i) {
          const auto ls_id = in->series_ids[i];
          if (lss.next_item_index() > ls_id) [[likely]] {
            auto in_label_set = lss[ls_id];
            auto& out_label_set = out->label_sets[i];
            out_label_set.reserve(in_label_set.size());
            std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                                   [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
          }
        }
      },
      *in->lss);
}

extern "C" void prompp_primitives_lss_free_label_sets(void* args) {
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Slice<PromPP::Primitives::Go::Label>> label_sets;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_query_label_names(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    uint32_t status{};
    GoSliceOfString names;
  };

  using LabelNamesQuerier = series_index::querier::LabelNamesQuerier<QueryableEncodingBimap>;

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();
  out->status = static_cast<uint32_t>(LabelNamesQuerier{std::get<QueryableEncodingBimap>(*in->lss)}.query(
      in->label_matchers, [out](std::string_view name) PROMPP_LAMBDA_INLINE { out->names.emplace_back(name); }));
}

extern "C" void prompp_primitives_lss_query_label_values(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::String label_name;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    uint32_t status{};
    GoSliceOfString values;
  };

  using LabelValuesQuerier = series_index::querier::LabelValuesQuerier<QueryableEncodingBimap>;

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();
  out->status = static_cast<uint32_t>(LabelValuesQuerier{std::get<QueryableEncodingBimap>(*in->lss)}.query(
      static_cast<std::string_view>(in->label_name), in->label_matchers,
      [out](std::string_view value) PROMPP_LAMBDA_INLINE { out->values.emplace_back(value); }));
}

extern "C" void prompp_primitives_lss_get_label_name_ids(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    GoSliceViewString names;
  };
  struct Result {
    PromPP::Primitives::Go::SliceView<uint32_t> ids;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  const auto& names_trie = std::get<QueryableEncodingBimap>(*in->lss).trie_index().names_trie();
  constexpr auto kMissingId = std::numeric_limits<uint32_t>::max();

  for (size_t i = 0; i < in->names.size(); ++i) {
    if (const auto id = names_trie.lookup(static_cast<std::string_view>(in->names[i]))) {
      out->ids[i] = *id;
    } else {
      out->ids[i] = kMissingId;
    }
  }
}

extern "C" void prompp_create_snapshot_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    SnapshotLSSVariantPtr snapshot;
  };

  new (res) Result{.snapshot = entrypoint::head::create_snapshot_lss(*static_cast<Arguments*>(args)->lss)};
}

extern "C" void prompp_primitives_snapshot_dtor(void* args) {
  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

using BitsetPtr = std::unique_ptr<BareBones::Bitset>;

extern "C" void prompp_primitives_lss_bitset_series(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    BitsetPtr bitset;
  };

  const auto& src = std::get<QueryableEncodingBimap>(*static_cast<Arguments*>(args)->lss);
  new (res) Result{.bitset = std::make_unique<BareBones::Bitset>(src.added_series())};
}

extern "C" void prompp_primitives_lss_bitset_dtor(void* args) {
  struct Arguments {
    BitsetPtr bitset;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_snapshot_lss_copy_added_series(uint64_t source_snapshot,
                                                                 uint64_t source_bitset,
                                                                 uint64_t destination_lss,
                                                                 uint64_t ids_mapping) {
  const auto& src_snapshot_variant = *std::bit_cast<entrypoint::head::SnapshotLSSVariant*>(source_snapshot);
  const auto& src = std::get<entrypoint::head::SnapshotLSS>(src_snapshot_variant);
  const auto& src_bitset = *std::bit_cast<BareBones::Bitset*>(source_bitset);
  auto& dst = std::get<QueryableEncodingBimap>(*std::bit_cast<entrypoint::head::LssVariant*>(destination_lss));
  const auto dst_src_ids_mapping = std::bit_cast<LsIdsSlicePtr*>(ids_mapping);
  *dst_src_ids_mapping = std::make_unique<LsIdsSlice>();

  series_index::QueryableEncodingBimapCopier copier(src, src.sorting_index(), src_bitset, dst, **dst_src_ids_mapping);
  copier.copy_added_series_and_build_indexes();
}

extern "C" void prompp_primitives_lss_set_pending_shrink_boundary(void* args) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t shrink_boundary;
  };
  const auto* in = static_cast<const Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  lss.set_pending_shrink_boundary(in->shrink_boundary);
}

extern "C" void prompp_primitives_lss_finalize_copy_and_shrink(void* args) {
  struct Arguments {
    LssVariantPtr lss;
    SnapshotLSSVariantPtr resolve_snapshot;
    LsIdsSlicePtr new_to_old_mapping;
  };
  const auto* in = static_cast<const Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  auto& resolve_snapshot = std::get<entrypoint::head::SnapshotLSS>(*in->resolve_snapshot);
  lss.finalize_copy_and_shrink(resolve_snapshot, *in->new_to_old_mapping);
}

void prompp_primitives_free_ls_ids_mapping(void* args) {
  struct Arguments {
    LsIdsSlicePtr ls_ids_mapping;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
