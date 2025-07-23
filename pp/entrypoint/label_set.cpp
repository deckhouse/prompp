#include "label_set.h"

#include "bare_bones/algorithm.h"
#include "bare_bones/iterator.h"
#include "entrypoint/head/lss.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"
#include "prometheus/value.h"

using entrypoint::head::LssVariantPtr;
using PromPP::Primitives::Go::Slice;
using PromPP::Primitives::Go::SliceView;

extern "C" void prompp_label_set_length(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    size_t length;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        const auto& ls = lss[in->series_id];
        const auto size_of_ls = ls.size();

        if (in->drop_metric_name && size_of_ls != 0) {
          for (const auto& label : ls) {
            if (label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
              continue;
            }

            ++out->length;
          }
        } else {
          out->length = ls.size();
        }
      },
      *in->lss);
}

extern "C" void prompp_label_set_serialize(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
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

        if (in->drop_metric_name) {
          for (const auto& label : in_label_set) {
            if (label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
              continue;
            }

            out_label_set.push_back(Label{.name = String{label.first}, .value = String{label.second}});
          }

          return;
        }

        std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                               [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
      },
      *in->lss);
}

extern "C" void prompp_label_set_free(void* args) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Label> label_set;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

namespace Bytes {

static constexpr uint8_t kLabelSeparator = '\xFE';
static constexpr uint8_t kNameValueSeparator = '\xFF';

class SizeCalculator {
 public:
  explicit SizeCalculator(bool drop_metric_name) : drop_metric_name_(drop_metric_name) {}

  template <class Label>
  PROMPP_ALWAYS_INLINE SizeCalculator& operator=(const Label& label) noexcept {
    operator()(label);
    return *this;
  }

  template <class Label>
  PROMPP_ALWAYS_INLINE void operator()(const Label& label) noexcept {
    if (drop_metric_name_ && label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
      return;
    }

    label_size_ += label.first.size() + label.second.size();
    ++label_count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept {
    uint32_t size = sizeof(kLabelSeparator);
    if (label_count_ > 0) {
      size += label_size_ + sizeof(kNameValueSeparator) * label_count_ * 2 - 1;
    }

    return size;
  }

 private:
  uint32_t label_size_{};
  uint32_t label_count_{};
  bool drop_metric_name_;
};

class Writer {
 public:
  explicit Writer(uint8_t* bytes, bool drop_metric_name) : bytes_(bytes), drop_metric_name_(drop_metric_name) { *bytes_++ = kLabelSeparator; }

  template <class Label>
  PROMPP_ALWAYS_INLINE Writer& operator=(const Label& label) noexcept {
    operator()(label);
    return *this;
  }

  template <class Label>
  PROMPP_ALWAYS_INLINE void operator()(const Label& label) noexcept {
    if (drop_metric_name_ && label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
      return;
    }

    if (++label_count_ > 1) [[likely]] {
      *bytes_++ = kNameValueSeparator;
    }

    std::memcpy(bytes_, label.first.data(), label.first.size());
    bytes_ += label.first.size();

    *bytes_++ = kNameValueSeparator;

    std::memcpy(bytes_, label.second.data(), label.second.size());
    bytes_ += label.second.size();
  }

  PROMPP_ALWAYS_INLINE uint32_t written_bytes(const uint8_t* start_bytes) const noexcept { return static_cast<uint32_t>(bytes_ - start_bytes); }

 private:
  uint8_t* bytes_;
  uint32_t label_count_{};
  bool drop_metric_name_;
};

};  // namespace Bytes

struct LabelNameLess {
  using Label = const std::pair<std::string_view, std::string_view>;

  PROMPP_ALWAYS_INLINE bool operator()(const Label& label, const PromPP::Primitives::Go::String& name) const noexcept {
    return label.first < static_cast<std::string_view>(name);
  }
  PROMPP_ALWAYS_INLINE bool operator()(const Label& a, const Label& b) const noexcept { return a.first < b.first; }
  PROMPP_ALWAYS_INLINE bool operator()(const PromPP::Primitives::Go::String& name, const Label& label) const noexcept {
    return static_cast<std::string_view>(name) < label.first;
  }
  PROMPP_ALWAYS_INLINE bool operator()(const PromPP::Primitives::Go::String& a, const PromPP::Primitives::Go::String& b) const noexcept { return a < b; }
};

extern "C" void prompp_label_set_bytes_size(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    uint32_t size;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        Bytes::SizeCalculator calculator(in->drop_metric_name);
        std::ranges::for_each(lss[in->series_id], std::ref(calculator));
        out->size = calculator.size();
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data(), in->drop_metric_name);
        std::ranges::for_each(lss[in->series_id], std::ref(writer));
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes_with_labels(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    SliceView<PromPP::Primitives::Go::String> names;
    bool drop_metric_name;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data(), in->drop_metric_name);
        std::ranges::set_intersection(lss[in->series_id], in->names, BareBones::iterator::OperationIterator(writer), LabelNameLess{});
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes_without_labels(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    SliceView<PromPP::Primitives::Go::String> names;
    bool drop_metric_name;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data(), in->drop_metric_name);
        std::ranges::set_difference(lss[in->series_id], in->names, BareBones::iterator::OperationIterator(writer), LabelNameLess{});
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}

extern "C" void prompp_label_set_get_value(void* args, void* res) {
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

extern "C" void prompp_label_set_has_label_name(void* args, void* res) {
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

extern "C" void prompp_label_set_has_duplicate_label_names(void* args, void* res) {
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    String label_name;
    bool is_has{};
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        const auto& in_label_set = lss[in->series_id];
        std::string_view prev_name{};
        for (const auto& label : in_label_set) {
          if (in->drop_metric_name && label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
            continue;
          }

          if (label.first == prev_name) {
            out->label_name = String(label.first);
            out->is_has = true;
            return;
          }
          prev_name = label.first;
        }
      },
      *in->lss);
}

extern "C" void prompp_label_set_hash(void* args, void* res) {
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);

  std::visit(
      [in, res](auto& lss) {
        new (res) Result{.hash = static_cast<uint64_t>(PromPP::Primitives::hash::hash_of_label_set(lss[in->series_id], in->drop_metric_name))};
      },
      *in->lss);
}

template <class Filter>
class CalculateHashIterator {
 public:
  explicit CalculateHashIterator(BareBones::XXHash* hash, Filter&& filter, bool drop_metric_name)
      : hash_(hash), filter_(filter), drop_metric_name_(drop_metric_name) {}

  using iterator_category = std::forward_iterator_tag;
  using difference_type = ptrdiff_t;

  CalculateHashIterator& operator*() noexcept { return *this; }
  CalculateHashIterator& operator++() noexcept { return *this; }
  CalculateHashIterator& operator++(int) noexcept { return *this; }
  PROMPP_ALWAYS_INLINE CalculateHashIterator& operator=(const PromPP::Primitives::LabelView& label) noexcept {
    if (filter_(label, drop_metric_name_)) {
      hash_->extend(label.first, label.second);
    }

    return *this;
  }
  CalculateHashIterator& operator=(const PromPP::Primitives::Go::String&) noexcept { return *this; }

 private:
  BareBones::XXHash* hash_;
  [[no_unique_address]] Filter filter_;
  bool drop_metric_name_;
};

struct LabelNameLessHash {
  using String = PromPP::Primitives::Go::String;
  using LabelView = PromPP::Primitives::LabelView;

  bool operator()(const LabelView& a, const LabelView& b) const noexcept { return a.first < b.first; }
  bool operator()(const LabelView& a, const String& b) const noexcept { return a.first < static_cast<std::string_view>(b); }
  bool operator()(const String& a, const LabelView& b) const noexcept { return static_cast<std::string_view>(a) < b.first; }
  bool operator()(const String& a, const String& b) const noexcept { return a < b; }
};

extern "C" void prompp_label_set_hash_for_labels(void* args, void* res) {
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    Slice<String> label_names;
    uint32_t series_id;
    bool drop_metric_name;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);

  BareBones::XXHash hash;
  std::visit(
      [in, &hash](auto& lss) {
        auto in_label_set = lss[in->series_id];
        std::ranges::set_intersection(in_label_set, in->label_names,
                                      CalculateHashIterator{&hash,
                                                            [](const auto& label, bool drop_metric_name) {
                                                              if (drop_metric_name && label.first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
                                                                return false;
                                                              }

                                                              return true;
                                                            },
                                                            in->drop_metric_name},
                                      LabelNameLessHash{});
      },
      *in->lss);
  new (res) Result{.hash = hash.hash()};
}

extern "C" void prompp_label_set_hash_without_labels(void* args, void* res) {
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
        std::ranges::set_difference(in_label_set, in->label_names,
                                    CalculateHashIterator{&hash,
                                                          [](const PromPP::Primitives::LabelView& label, bool drop_metric_name [[maybe_unused]]) {
                                                            return label.first != PromPP::Prometheus::kMetricLabelName;
                                                          },
                                                          false},
                                    LabelNameLessHash{});
      },
      *in->lss);
  new (res) Result{.hash = hash.hash()};
}

extern "C" void prompp_label_set_equal(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss_a;
    LssVariantPtr lss_b;
    uint32_t series_id_a;
    uint32_t series_id_b;
    bool drop_metric_name_a;
    bool drop_metric_name_b;
  };
  struct Result {
    bool is_equal;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss_a, auto& lss_b) {
        if (in->drop_metric_name_a || in->drop_metric_name_b) {
          const auto& ls_a = lss_a[in->series_id_a];
          const auto& ls_b = lss_b[in->series_id_b];

          auto it_a = ls_a.begin();
          auto it_b = ls_b.begin();

          while (it_a != ls_a.end() && it_b != ls_b.end()) {
            if (in->drop_metric_name_a && (*it_a).first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
              ++it_a;
            }

            if (in->drop_metric_name_b && (*it_b).first == PromPP::Prometheus::kMetricLabelName) [[unlikely]] {
              ++it_b;
            }

            if (*it_a != *it_b) {
              out->is_equal = false;
              return;
            }
            ++it_a;
            ++it_b;
          }

          if (it_a != ls_a.end() || it_b != ls_b.end()) [[unlikely]] {
            out->is_equal = false;
            return;
          }

          out->is_equal = true;
          return;
        }

        out->is_equal = lss_a[in->series_id_a] == lss_b[in->series_id_b];
      },
      *in->lss_a, *in->lss_b);
}

extern "C" void prompp_label_set_compare(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss_a;
    LssVariantPtr lss_b;
    uint32_t series_id_a;
    uint32_t series_id_b;
    bool drop_metric_name_a;
    bool drop_metric_name_b;
  };
  struct Result {
    int64_t result;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss_a, auto& lss_b) {
        if (auto result = BareBones::lexicographical_compare_three_way(lss_a[in->series_id_a], lss_b[in->series_id_b], in->drop_metric_name_a,
                                                                       in->drop_metric_name_b, std::compare_three_way{});
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

extern "C" void prompp_label_set_from_builder_hash(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr readonly_lss;
    SliceView<PromPP::Primitives::Go::Label> sorted_add;
    SliceView<PromPP::Primitives::Go::String> sorted_del;
    uint32_t ls_id;
  };
  struct Result {
    uint64_t hash;
  };

  auto in = static_cast<Arguments*>(args);
  static const entrypoint::head::ReadonlyLss::value_type empty_label_set;
  const auto& label_set = in->readonly_lss ? std::get<entrypoint::head::ReadonlyLss>(*in->readonly_lss)[in->ls_id] : empty_label_set;

  new (res) Result{.hash = hash_value(LabelSetBuilder{label_set, in->sorted_add, in->sorted_del})};
}

extern "C" void prompp_label_set_equal_with_builder(void* args, void* res) {
  using PromPP::Primitives::Go::LabelSetBuilder;
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LssVariantPtr snapshot;
    LssVariantPtr builder_snapshot;
    SliceView<PromPP::Primitives::Go::Label> builder_sorted_add;
    SliceView<PromPP::Primitives::Go::String> builder_sorted_del;
    uint32_t builder_ls_id;
    uint32_t ls_id;
  };
  struct Result {
    bool eq;
  };

  auto in = static_cast<Arguments*>(args);

  const auto& label_set = std::get<entrypoint::head::ReadonlyLss>(*in->snapshot)[in->ls_id];

  static const entrypoint::head::ReadonlyLss::value_type empty_label_set;
  const auto& builder_label_set = in->builder_snapshot ? std::get<entrypoint::head::ReadonlyLss>(*in->builder_snapshot)[in->builder_ls_id] : empty_label_set;

  new (res) Result{.eq = label_set == LabelSetBuilder{builder_label_set, in->builder_sorted_add, in->builder_sorted_del}};
}
