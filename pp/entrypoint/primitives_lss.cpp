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

extern "C" void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    LssType lss_type;
  };
  struct Result {
    LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(static_cast<Arguments*>(args)->lss_type)};
}

extern "C" void prompp_primitives_lss_copy_added_series(void* args) {
  struct Arguments {
    LssVariantPtr source;
    LssVariantPtr destination;
  };

  const auto arguments = static_cast<Arguments*>(args);
  std::get<QueryableEncodingBimap>(*arguments->source).copy_added_series(std::get<QueryableEncodingBimap>(*arguments->destination));
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
    entrypoint::head::lss_memory::has_reallocations = false;
    const auto ls_id = lss.find_or_emplace(label_set);
    return {.ls_id = ls_id, .lss_has_reallocations = entrypoint::head::lss_memory::has_reallocations};
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
        return find_or_emplace<Lss>(lss, LabelSetBuilder{std::get<entrypoint::head::ReadonlyQueryableEncodingBimap>(*builder.readonly_lss)[builder.ls_id],
                                                         builder.sorted_add, builder.sorted_del});
      },
      *in->lss));
}

struct LssQueryResult {
  PromPP::Primitives::Go::Slice<uint32_t> matches;
  PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths;
  uint32_t status;
};

extern "C" void prompp_primitives_lss_query(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    GoLabelMatchers label_matchers;
    series_index::QueriedSeries::Source query_source;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<uint32_t> matches;
    PromPP::Primitives::Go::Slice<uint16_t> label_set_lengths{};
    LssVariantPtr lss_copy;
    uint32_t status;
  };

  using Querier = series_index::querier::Querier<QueryableEncodingBimap, PromPP::Primitives::Go::Slice>;

  const auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);
  auto query_result = Querier{lss}.query(in->label_matchers);
  lss.sort_series_ids(query_result.series_ids);
  lss.set_queried_series(in->query_source, query_result.series_ids);

  const auto out = new (res) Result{
      .matches = std::move(query_result.series_ids),
      .lss_copy = entrypoint::head::create_readonly_lss(*in->lss),
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
