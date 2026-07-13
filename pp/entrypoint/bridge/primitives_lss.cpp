#include "primitives_lss.h"
#include "annotations.h"

#include <limits>

#include "bare_bones/bitset.h"
#include "bare_bones/vector.h"
#include "entrypoint/types/lss.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"
#include "series_index/querier/label_names_querier.h"
#include "series_index/querier/label_values_querier.h"
#include "series_index/querier/series_operations.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using GoLabelMatchers = PromPP::Primitives::Go::SliceView<PromPP::Prometheus::LabelMatcherTrait<PromPP::Primitives::Go::String>>;
using GoSliceOfString = PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::String>;
using GoSliceViewString = PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::String>;
using entrypoint::types::LsIdsSlice;
using entrypoint::types::LsIdsSlicePtr;
using entrypoint::types::LssType;
using entrypoint::types::LssVariantPtr;
using entrypoint::types::QueryableEncodingBimap;
using entrypoint::types::SnapshotLSSVariantPtr;

struct FindOrEmplaceResult {
  uint32_t ls_id;
  bool lss_has_reallocations;
};

template <class Lss>
PROMPP_ALWAYS_INLINE FindOrEmplaceResult find_or_emplace(auto& lss, const auto& label_set) {
  if constexpr (Lss::kIsReadOnly) {
    throw BareBones::Exception(0x1b877a0ab46a69a6, "lss is readonly");
  } else {
    const entrypoint::types::ReallocationsDetector reallocation_detector(lss);
    const auto ls_id = lss.find_or_emplace(label_set);
    return {.ls_id = ls_id, .lss_has_reallocations = reallocation_detector.has_reallocations()};
  }
}

struct LssQueryResult {
  PromPP::Primitives::Go::Slice<uint32_t> matches;
  PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths;
  uint32_t status;
};

using Querier = series_index::querier::Querier<PromPP::Primitives::Go::Slice>;
using SelectorPtr = std::unique_ptr<Querier::Selector>;

struct GroupSeriesByLabelNamesResult {
  PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::Slice<uint32_t>> groups;
};

using BitsetPtr = std::unique_ptr<BareBones::Bitset>;

}  // namespace

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     lss_type uint32 // type of lss;
 * }
 *
 * @param res {
 *     lss uintptr     // pointer to constructed label sets;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    LssType lss_type;
  };
  struct Result {
    LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(static_cast<Arguments*>(args)->lss_type)};
}

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    LssVariantPtr lss;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    uint64_t allocated_memory;
  };

  std::visit([res](const auto& lss) { new (res) Result{.allocated_memory = lss.allocated_memory()}; }, *static_cast<Arguments*>(args)->lss);
}

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss uintptr              // pointer to constructed lss;
 *     label_set model.LabelSet // label set
 * }
 *
 * @param res {
 *     ls_id uint32                  // inserted (or found) label set id
 *     bool  lss_has_reallocations   // true if lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_find_or_emplace(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  using Result = FindOrEmplaceResult;

  auto in = static_cast<Arguments*>(args);
  new (res) Result(std::visit([in]<typename Lss>(Lss& lss) { return find_or_emplace<Lss>(lss, in->label_set); }, *in->lss));
}

/**
 * @brief insert label set builder into lss
 *
 * @param args {
 *     lss uintptr                    // pointer to constructed lss;
 *     builder struct {
 *        snapshot     uintptr        // pointer to constructed snapshot lss;
 *        ls_id        uint32         // series id
 *        sorted_add   []model.Label  // slice of sorted by name labels
 *        sorted_del   []string       // slice of sorted label names
 *     }
 * }
 *
 * @param res {
 *     ls_id uint32                   // inserted (or found) label set id
 *     bool  lss_has_reallocations    // true if lss has reallocations
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_find_or_emplace_builder(void* args, void* res) {
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
  using Result = FindOrEmplaceResult;

  const auto in = static_cast<Arguments*>(args);
  new (res) Result(std::visit(
      [&builder = in->builder]<typename Lss>(Lss& lss) {
        static const entrypoint::types::SnapshotLSS::value_type empty_label_set;
        const auto& label_set = builder.snapshot ? std::get<entrypoint::types::SnapshotLSS>(*builder.snapshot)[builder.ls_id] : empty_label_set;

        return find_or_emplace<Lss>(lss, LabelSetBuilder{label_set, builder.sorted_add, builder.sorted_del});
      },
      *in->lss));
}

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     selector uintptr // constructed selector
 *     status   uint32  // query status
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_query_selector(void* args, void* res) {
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

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     snapshot uintptr // pointer to snapshot
 *     selector uintptr // pointer to constructed selector
 * }
 *
 * @param res {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_snapshot_query(void* args, void* res) {
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

/**
 * @brief group series by label name ids
 *
 * @param args {
 *     snapshot       uintptr  // pointer to snapshot
 *     series_ids     []uint32 // series_ids for grouping
 *     label_name_ids []uint32 // label names ids for grouping
 * }
 *
 * @param res {
 *     groups [][]uint32 // grouped series
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_group_series_by_label_names(void* args, void* res) {
  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
    PromPP::Primitives::Go::SliceView<uint32_t> label_name_ids;
  };
  using Result = GroupSeriesByLabelNamesResult;

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  series_index::querier::group_series_by_label_names<entrypoint::types::SnapshotLSS, PromPP::Primitives::Go::Slice>(
      std::get<entrypoint::types::SnapshotLSS>(*in->snapshot), in->series_ids.span(), in->label_name_ids.span(), out->groups);
}

/**
 * @brief free groups returned by prompp_primitives_group_series_by_label_names
 *
 * @param args {
 *     groups [][]uint32 // grouped series (same layout as Result.groups)
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_group_series_by_label_names_result_free(void* args) {
  using Arguments = GroupSeriesByLabelNamesResult;

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief free label set matches returned by prompp_primitives_snapshot_query
 *
 * @param args {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_query_result_free(void* args) {
  using Arguments = LssQueryResult;

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief get label sets by series id
 *
 * @param args {
 *     lss uintptr    // pointer to constructed lss;
 *     ls_id []uint32 // series ids
 * }
 *
 * @param res {
 *     label_sets [][]struct {key, value String} // label sets
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_get_label_sets(void* args, void* res) {
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

/**
 * @brief free label sets returned by prompp_primitives_lss_get_label_sets
 *
 * @param args {
 *     label_sets [][]struct {key, value String} // label set
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_free_label_sets(void* args) {
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Slice<PromPP::Primitives::Go::Label>> label_sets;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     names  []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_query_label_names(void* args, void* res) {
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

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_name string                   // label name
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     values []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_query_label_values(void* args, void* res) {
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

/**
 * @brief Resolve label name strings to ids for a queryable LSS.
 *
 * @param args {
 *     lss    uintptr    // pointer to constructed queryable lss
 *     names  []string   // label names
 * }
 *
 * @param res {
 *     ids []uint32  // snapshot of lss
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_get_label_name_ids(void* args, void* res) {
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

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                 // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     snapshot          uintptr  // snapshot of lss
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_create_snapshot_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    SnapshotLSSVariantPtr snapshot;
  };

  new (res) Result{.snapshot = entrypoint::types::create_snapshot_lss(*static_cast<Arguments*>(args)->lss)};
}

/**
 * @brief Destroy Primitives snapshot LSS.
 *
 * @param args {
 *     snapshot uintptr // pointer of snapshot;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_snapshot_dtor(void* args) {
  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief returns a copy of the bitset of added series from the lss.
 *
 * @param args {
 *    lss              uintptr  // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     bitset          uintptr  // bitset of added series;
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_bitset_series(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    BitsetPtr bitset;
  };

  const auto& src = std::get<QueryableEncodingBimap>(*static_cast<Arguments*>(args)->lss);
  new (res) Result{.bitset = std::make_unique<BareBones::Bitset>(src.added_series())};
}

/**
 * @brief destroy bitset of added series.
 *
 * @param args {
 *     bitset          uintptr  // bitset of added series;
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_bitset_dtor(void* args) {
  struct Arguments {
    BitsetPtr bitset;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Copy the label sets from the source lss to the destination lss that were added source lss.
 *
 * @param source_snapshot pointer to source snapshot;
 * @param source_bitset pointer to source bitset;
 * @param destination_lss pointer to destination label sets;
 * @param ids_mapping pointer to uintptr
 *
 * @attention This binding used as a CGO call!!!
 *
 */
extern "C" PROMPP(entrypoint, cgo) void prompp_primitives_snapshot_lss_copy_added_series(uint64_t source_snapshot,
                                                                 uint64_t source_bitset,
                                                                 uint64_t destination_lss,
                                                                 uint64_t ids_mapping) {
  const auto& src_snapshot_variant = *std::bit_cast<entrypoint::types::SnapshotLSSVariant*>(source_snapshot);
  const auto& src = std::get<entrypoint::types::SnapshotLSS>(src_snapshot_variant);
  const auto& src_bitset = *std::bit_cast<BareBones::Bitset*>(source_bitset);
  auto& dst = std::get<QueryableEncodingBimap>(*std::bit_cast<entrypoint::types::LssVariant*>(destination_lss));
  const auto dst_src_ids_mapping = std::bit_cast<LsIdsSlicePtr*>(ids_mapping);
  *dst_src_ids_mapping = std::make_unique<LsIdsSlice>();

  series_index::QueryableEncodingBimapCopier copier(src, src.sorting_index(), src_bitset, dst, **dst_src_ids_mapping);
  copier.copy_added_series_and_build_indexes();
}

/**
 * @brief set pending shrink boundary on LSS (switch to "fixed" state before snapshot and copy).
 *
 * @param args {
 *     lss                 uintptr  // pointer to source queryable lss;
 *     shrink_boundary      uint32  // boundary
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_set_pending_shrink_boundary(void* args) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t shrink_boundary;
  };
  const auto* in = static_cast<const Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  lss.set_pending_shrink_boundary(in->shrink_boundary);
}

/**
 * @brief Shrink current lss to checkpoint and set post-shrink mapping and copy pointers.
 *
 * @param args {
 *     lss                uintptr  // pointer to source queryable lss;
 *     resolve_snapshot   uintptr  // pointer to snapshot lss for resolving ids with mapping;
 *     new_to_old_mapping uintptr  // pointer to ls id `new (copy) -> old (source)` mapping from copier
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_lss_finalize_copy_and_shrink(void* args) {
  struct Arguments {
    LssVariantPtr lss;
    SnapshotLSSVariantPtr resolve_snapshot;
    LsIdsSlicePtr new_to_old_mapping;
  };
  const auto* in = static_cast<const Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  auto& resolve_snapshot = std::get<entrypoint::types::SnapshotLSS>(*in->resolve_snapshot);
  lss.finalize_copy_and_shrink(resolve_snapshot, *in->new_to_old_mapping);
}

/**
 * @brief destroy ls ids mapping
 *
 * @param args {
 *     ls_ids_mapping uintptr
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_primitives_free_ls_ids_mapping(void* args) {
  struct Arguments {
    LsIdsSlicePtr ls_ids_mapping;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
