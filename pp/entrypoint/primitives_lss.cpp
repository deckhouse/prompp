#include "primitives_lss.h"

#include "bare_bones/xxhash.h"
#include "hashdex.hpp"
#include "head/lss.h"
#include "primitives/go_slice.h"
#include "prometheus/value.h"
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

extern "C" void prompp_primitives_lss_find_or_emplace(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  struct Result {
    uint32_t ls_id;
  };

  auto in = static_cast<Arguments*>(args);
  new (res) Result{.ls_id = std::visit(
                       [in]<typename Lss>(Lss& lss) -> PromPP::Primitives::LabelSetID {
                         if constexpr (Lss::kIsReadOnly) {
                           throw BareBones::Exception(0x1b877a0ab46a69a6, "lss is readonly");
                         } else {
                           return lss.find_or_emplace(in->label_set);
                         }
                       },
                       *in->lss)};
}

extern "C" void prompp_primitives_lss_find_or_emplace_label_set(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  struct Result {
    LssVariantPtr lss_ro_ptr;
    uint32_t ls_id;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);

  out->ls_id = lss.find_or_emplace(in->label_set);
  out->lss_ro_ptr = entrypoint::head::create_lss_readonly(lss);
}

extern "C" void prompp_primitives_lss_find(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  struct Result {
    LssVariantPtr lss_ro_ptr;
    uint32_t ls_id;
    bool has;
  };

  auto in = static_cast<Arguments*>(args);
  auto& lss = std::get<QueryableEncodingBimap>(*in->lss);

  std::optional<uint32_t> ls_id = lss.find(in->label_set);

  if (ls_id.has_value()) {
    new (res) Result{.lss_ro_ptr = entrypoint::head::create_lss_readonly(lss), .ls_id = ls_id.value(), .has = lss.find(in->label_set).has_value()};
  }
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
      .lss_copy = entrypoint::head::create_lss_readonly(lss),
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

//
// label_sets
//

extern "C" void prompp_primitives_label_set_length(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    size_t length;
  };

  auto in = static_cast<Arguments*>(args);

  std::visit([in, res](auto& lss) { new (res) Result{.length = lss[in->series_id].size()}; }, *in->lss);
}

extern "C" void prompp_primitives_label_set_serialize(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    Slice<Label> label_set;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        auto in_label_set = lss[in->series_id];
        auto& out_label_set = out->label_set;
        out_label_set.reserve(in_label_set.size());
        std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                               [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
      },
      *in->lss);
}

extern "C" void prompp_primitives_label_set_free(void* args) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Label> label_set;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_label_set_get_value(void* args, void* res) {
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    String label_name;
    uint32_t series_id;
  };
  struct Result {
    String label_value;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        auto in_label_set = lss[in->series_id];
        for (const auto& [ln, lv] : in_label_set) {
          if (ln == in->label_name) {
            out->label_value = String{lv};
            return;
          }
        }
      },
      *in->lss);
}

extern "C" void prompp_primitives_label_set_has_label_name(void* args, void* res) {
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    String label_name;
    uint32_t series_id;
  };
  struct Result {
    bool is_has{};
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        auto in_label_set = lss[in->series_id];
        for (const auto& label : in_label_set) {
          if (String{label.first} == in->label_name) {
            out->is_has = true;
            return;
          }
        }
      },
      *in->lss);
}

extern "C" void prompp_primitives_label_set_hash(void* args, void* res) {
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);

  std::visit([in, res](auto& lss) { new (res) Result{.hash = static_cast<uint64_t>(PromPP::Primitives::hash::hash_of_label_set(lss[in->series_id]))}; },
             *in->lss);
}

template <class Filter>
class CalculateHashIterator {
 public:
  explicit CalculateHashIterator(BareBones::XXHash* hash, Filter&& filter) : hash_(hash), filter_(filter) {}

  using iterator_category = std::forward_iterator_tag;
  using difference_type = ptrdiff_t;

  CalculateHashIterator& operator*() noexcept { return *this; }
  CalculateHashIterator& operator++() noexcept { return *this; }
  CalculateHashIterator& operator++(int) noexcept { return *this; }
  PROMPP_ALWAYS_INLINE CalculateHashIterator& operator=(const PromPP::Primitives::LabelView& label) noexcept {
    if (filter_(label)) {
      hash_->extend(label.first, label.second);
    }

    return *this;
  }
  CalculateHashIterator& operator=(const PromPP::Primitives::Go::String&) noexcept { return *this; }

 private:
  BareBones::XXHash* hash_;
  [[no_unique_address]] Filter filter_;
};

struct LabelNameLess {
  using String = PromPP::Primitives::Go::String;
  using LabelView = PromPP::Primitives::LabelView;

  bool operator()(const LabelView& a, const LabelView& b) const noexcept { return a.first < b.first; }
  bool operator()(const LabelView& a, const String& b) const noexcept { return a.first < static_cast<std::string_view>(b); }
  bool operator()(const String& a, const LabelView& b) const noexcept { return static_cast<std::string_view>(a) < b.first; }
  bool operator()(const String& a, const String& b) const noexcept { return a < b; }
};

extern "C" void prompp_primitives_label_set_hash_for_labels(void* args, void* res) {
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    Slice<String> label_names;
    uint32_t series_id;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);

  BareBones::XXHash hash;
  std::visit(
      [in, &hash](auto& lss) {
        auto in_label_set = lss[in->series_id];
        std::ranges::set_intersection(in_label_set, in->label_names, CalculateHashIterator{&hash, [](const auto&) { return true; }}, LabelNameLess{});
      },
      *in->lss);
  new (res) Result{.hash = hash.hash()};
}

extern "C" void prompp_primitives_label_set_hash_without_labels(void* args, void* res) {
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    Slice<String> label_names;
    uint32_t series_id;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);

  BareBones::XXHash hash;
  std::visit(
      [in, &hash](auto& lss) {
        auto in_label_set = lss[in->series_id];
        std::ranges::set_difference(
            in_label_set, in->label_names,
            CalculateHashIterator{&hash, [](const PromPP::Primitives::LabelView& label) { return label.first != PromPP::Prometheus::kMetricLabelName; }},
            LabelNameLess{});
      },
      *in->lss);
  new (res) Result{.hash = hash.hash()};
}

extern "C" void prompp_primitives_label_set_equal(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss_a;
    LssVariantPtr lss_b;
    uint32_t series_id_a;
    uint32_t series_id_b;
  };
  struct Result {
    bool is_equal;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit([in, out](auto& lss_a, auto& lss_b) { out->is_equal = lss_a[in->series_id_a] == lss_b[in->series_id_b]; }, *in->lss_a, *in->lss_b);
}

extern "C" void prompp_primitives_label_set_compare(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss_a;
    LssVariantPtr lss_b;
    uint32_t series_id_a;
    uint32_t series_id_b;
  };
  struct Result {
    int64_t result;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss_a, auto& lss_b) {
        if (auto result = BareBones::lexicographical_compare_three_way(lss_a[in->series_id_a], lss_b[in->series_id_b], std::compare_three_way{});
            std::is_lt(result)) {
          out->result = -1;
        } else if (std::is_eq(result)) {
          out->result = 0;
        } else {
          out->result = 1;
        }
      },
      *in->lss_a, *in->lss_b);
}
