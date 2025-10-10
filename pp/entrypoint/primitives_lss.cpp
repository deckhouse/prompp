#include "primitives_lss.h"

#include "bare_bones/xxhash.h"
#include "hashdex.hpp"
#include "head/lss.h"
#include "primitives/go_slice.h"
#include "series_index/querier/label_names_querier.h"
#include "series_index/querier/label_values_querier.h"

using GoLabelMatchers = PromPP::Primitives::Go::SliceView<PromPP::Prometheus::LabelMatcherTrait<PromPP::Primitives::Go::String>>;
using GoSliceOfString = PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::String>;
using entrypoint::head::LssType;
using entrypoint::head::LssVariantPtr;
using entrypoint::head::QueryableEncodingBimap;

using Bitset = BareBones::Bitset;
using BitsetPtr = std::unique_ptr<Bitset>;

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
      LssVariantPtr readonly_lss;
      uint32_t ls_id;
      SliceView<PromPP::Primitives::Go::Label> sorted_add;
      SliceView<PromPP::Primitives::Go::String> sorted_del;
    } builder;
  };

  const auto in = static_cast<Arguments*>(args);

  new (res) FindOrEmplaceResult(std::visit(
      [&builder = in->builder]<typename Lss>(Lss& lss) {
        static const entrypoint::head::ReadonlyLss::value_type empty_label_set;
        const auto& label_set = builder.readonly_lss ? std::get<entrypoint::head::ReadonlyLss>(*builder.readonly_lss)[builder.ls_id] : empty_label_set;

        return find_or_emplace<Lss>(lss, LabelSetBuilder{label_set, builder.sorted_add, builder.sorted_del});
      },
      *in->lss));
}

extern "C" void prompp_primitives_lss_find_or_emplace_from_builder(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr lss;
    LssVariantPtr readonly_lss;
    BitsetPtr bitset;
    SliceView<PromPP::Primitives::Go::Label> sorted_add;
    SliceView<PromPP::Primitives::Go::String> sorted_del;
    uint32_t ls_id;
  };

  struct Result {
    LssVariantPtr lss_ro_ptr;
    uint32_t ls_id;
    size_t length;
    bool lss_has_reallocations;
  };

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out]<typename Lss>(Lss& lss) {
        static const entrypoint::head::ReadonlyLss::value_type empty_label_set;
        const auto& label_set = in->readonly_lss ? std::get<entrypoint::head::ReadonlyLss>(*in->readonly_lss)[in->ls_id] : empty_label_set;

        const auto result = find_or_emplace<Lss>(lss, LabelSetBuilder{label_set, in->sorted_add, in->sorted_del});
        if (in->bitset != nullptr) [[likely]] {
          if (result.ls_id >= in->bitset->size()) [[unlikely]] {
            in->bitset->resize(result.ls_id + 1);
          }
          in->bitset->set(result.ls_id);
        }

        out->ls_id = result.ls_id;
        out->length = lss[out->ls_id].size();
        out->lss_has_reallocations = result.lss_has_reallocations;
      },
      *in->lss);

  if (out->lss_has_reallocations) [[unlikely]] {
    out->lss_ro_ptr = entrypoint::head::create_readonly_lss(*in->lss);
  }
}

extern "C" void prompp_primitives_lss_find_from_builder(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr lss;
    LssVariantPtr readonly_lss;
    SliceView<PromPP::Primitives::Go::Label> sorted_add;
    SliceView<PromPP::Primitives::Go::String> sorted_del;
    uint32_t ls_id;
  };
  struct Result {
    size_t length;
    uint32_t ls_id;
    bool has{false};
  };

  auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  static const entrypoint::head::ReadonlyLss::value_type empty_label_set;
  const auto& label_set = in->readonly_lss ? std::get<entrypoint::head::ReadonlyLss>(*in->readonly_lss)[in->ls_id] : empty_label_set;

  std::optional<uint32_t> ls_id = lss.find(LabelSetBuilder{label_set, in->sorted_add, in->sorted_del});

  if (ls_id.has_value()) {
    new (res) Result{.length = lss[ls_id.value()].size(), .ls_id = ls_id.value(), .has = ls_id.has_value()};
  }
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

extern "C" void prompp_primitives_lss_query(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    SelectorPtr selector;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<uint32_t> matches;
    PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths{};
    uint32_t status;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<entrypoint::head::ReadonlyLss>(*in->lss);
  auto query_result = Querier{}.query(*in->selector);
  in->selector.reset();
  lss.sorting_index().sort(query_result.series_ids);

  const auto out = new (res) Result{
      .matches = std::move(query_result.series_ids),
      .status = static_cast<uint32_t>(query_result.status),
  };
  out->label_set_lengths.reserve(out->matches.size());
  std::ranges::transform(out->matches, std::back_inserter(out->label_set_lengths),
                         [&lss](const auto ls_id) PROMPP_LAMBDA_INLINE { return static_cast<uint16_t>(lss[ls_id].size()); });
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
          if (lss.size() > ls_id) [[likely]] {
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

extern "C" void prompp_create_readonly_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    LssVariantPtr lss_copy;
  };

  new (res) Result{.lss_copy = entrypoint::head::create_readonly_lss(*static_cast<Arguments*>(args)->lss)};
}

extern "C" void prompp_primitives_lss_copy_added_series(uint64_t source_lss, uint64_t destination_lss) {
  auto& src = std::get<QueryableEncodingBimap>(*std::bit_cast<entrypoint::head::LssVariant*>(source_lss));
  auto& dst = std::get<QueryableEncodingBimap>(*std::bit_cast<entrypoint::head::LssVariant*>(destination_lss));
  src.build_deferred_indexes();

  series_index::QueryableEncodingBimapCopier copier(src, src.sorting_index(), src.added_series(), dst);
  copier.copy_added_series_and_build_indexes();
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

extern "C" void prompp_primitives_readonly_lss_copy_added_series(uint64_t source_lss, uint64_t source_bitset, uint64_t destination_lss) {
  const auto& src = std::get<entrypoint::head::ReadonlyLss>(*std::bit_cast<entrypoint::head::LssVariant*>(source_lss));
  const auto& src_bitset = *std::bit_cast<BareBones::Bitset*>(source_bitset);
  auto& dst = std::get<QueryableEncodingBimap>(*std::bit_cast<entrypoint::head::LssVariant*>(destination_lss));

  series_index::QueryableEncodingBimapCopier copier(src, src.sorting_index(), src_bitset, dst);
  copier.copy_added_series_and_build_indexes();
}

//
// Bitset
//

extern "C" void prompp_primitives_bitset_ctor(void* res) {
  struct Result {
    BitsetPtr bitset;
  };

  new (res) Result{.bitset = std::make_unique<Bitset>()};
}

extern "C" void prompp_primitives_bitset_dtor(void* args) {
  struct Arguments {
    BitsetPtr bitset;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_with_snapshot_stats(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr lss;
    BitsetPtr bitset;
    bool reset;
  };

  struct Result {
    uint64_t allocated_memory;
    size_t lss_size;
    uint32_t bitset_count{0};
  };

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out]<typename Lss>(Lss& lss) {
        out->lss_size = lss.size();

        if (!in->reset) [[unlikely]] {
          out->allocated_memory = lss.allocated_memory();
        }
      },
      *in->lss);

  if (in->bitset != nullptr) [[likely]] {
    out->bitset_count = in->bitset->popcount();

    if (in->reset) [[unlikely]] {
      in->bitset->clear();
    }
  }
}
