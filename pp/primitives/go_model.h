#pragma once

#include <string_view>

#include "primitives/go_slice.h"
#include "primitives/hash.h"
#include "primitives/primitives.h"

#include "bare_bones/exception.h"

namespace PromPP::Primitives::Go {

struct StringView {
  uint32_t begin;
  uint32_t length;
};

struct LabelView {
  StringView name;
  StringView value;
};

struct Label {
  union {
    String name;
    String first;
  };

  union {
    String value;
    String second;
  };
};

struct LabelSet {
  class IteratorSentinel {};

  template <class Derived>
  class PairsIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using difference_type = ptrdiff_t;

    explicit PairsIterator(const LabelSet* label_set) : label_set_(label_set), iterator_(label_set->pairs.begin()) {}

    PROMPP_ALWAYS_INLINE Derived& operator++() noexcept {
      ++iterator_;
      return *static_cast<Derived*>(this);
    }

    PROMPP_ALWAYS_INLINE Derived operator++(int) noexcept {
      auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return iterator_ == label_set_->pairs.end(); }

   protected:
    const LabelSet* label_set_;
    SliceView<LabelView>::const_iterator iterator_;
  };

  class Iterator : public PairsIterator<Iterator> {
   public:
    using PairsIterator::difference_type;
    using PairsIterator::iterator_category;
    using PairsIterator::PairsIterator;
    using value_type = std::pair<std::string_view, std::string_view>;
    using pointer = value_type*;
    using reference = value_type&;

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept {
      return {{label_set_->data.data() + iterator_->name.begin, iterator_->name.length},
              {label_set_->data.data() + iterator_->value.begin, iterator_->value.length}};
    }
  };

  struct Names {
    class NamesIterator : public PairsIterator<NamesIterator> {
     public:
      using PairsIterator::difference_type;
      using PairsIterator::iterator_category;
      using PairsIterator::PairsIterator;
      using value_type = std::string_view;
      using pointer = value_type*;
      using reference = value_type&;

      [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept {
        return {label_set_->data.data() + iterator_->name.begin, iterator_->name.length};
      }
    };

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return label_set->pairs.size(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return NamesIterator(label_set); }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    PROMPP_ALWAYS_INLINE friend size_t hash_value(const Names& label_set_names) noexcept { return hash::hash_of_string_list(label_set_names); }

    const LabelSet* label_set;
  };

  SliceView<char> data;
  SliceView<LabelView> pairs;

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return Iterator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return pairs.size(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto names() const noexcept { return Names{.label_set = this}; }

  PROMPP_ALWAYS_INLINE friend size_t hash_value(const LabelSet& label_set) noexcept { return hash::hash_of_label_set(label_set); }
};

template <class LabelSet, template <class> class AddContainer, template <class> class DelContainer>
class LabelSetBuilder {
 public:
  class IteratorSentinel {};

  template <class LabelIterator, class LabelEndIterator, class AddIterator, class DelIterator>
  class GenericIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using difference_type = ptrdiff_t;

    explicit GenericIterator(const LabelSetBuilder& builder)
        : label_iterator_(builder.label_set_.begin()),
          label_end_iterator_(builder.label_set_.end()),
          add_iterator_(builder.add_.begin()),
          add_end_iterator_(builder.add_.end()),
          del_iterator_(builder.del_.begin()),
          del_end_iterator_(builder.del_.end()) {
      next();
    }

    bool operator==(const IteratorSentinel&) const { return label_.first.empty(); }

   protected:
    Primitives::LabelView label_;

    void next() noexcept {
      label_.first = {};

      do {
        if (label_iterator_ != label_end_iterator_) {
          next_with_label();
        } else if (add_iterator_ != add_end_iterator_) {
          set_label(add_iterator_);
        } else {
          break;
        }
      } while (label_.first.empty());
    }

   private:
    LabelIterator label_iterator_;
    [[no_unique_address]] LabelEndIterator label_end_iterator_;
    AddIterator add_iterator_;
    AddIterator add_end_iterator_;
    DelIterator del_iterator_;
    DelIterator del_end_iterator_;

    PROMPP_ALWAYS_INLINE void next_with_label() noexcept {
      if (add_iterator_ != add_end_iterator_) {
        next_with_label_and_add();
      } else {
        set_label(label_iterator_);
      }
    }

    PROMPP_ALWAYS_INLINE void next_with_label_and_add() noexcept {
      const auto cmp_result = (*label_iterator_).first <=> static_cast<std::string_view>(add_iterator_->name);

      if (std::is_lt(cmp_result)) {
        set_label(label_iterator_);
      } else {
        if (std::is_eq(cmp_result)) {
          ++label_iterator_;
        }
        set_label(add_iterator_);
      }
    }

    template <class Iterator>
    PROMPP_ALWAYS_INLINE void set_label(Iterator& iterator) {
      const auto& label = *iterator;

      if (const auto name_view = static_cast<std::string_view>(label.first); !is_deleted(name_view)) {
        label_.first = name_view;
        label_.second = static_cast<std::string_view>(label.second);
      }

      ++iterator;
    }

    PROMPP_ALWAYS_INLINE bool is_deleted(std::string_view name) noexcept {
      while (del_iterator_ != del_end_iterator_) {
        if (const auto cmp_result = static_cast<std::string_view>(*del_iterator_) <=> name; std::is_lt(cmp_result)) {
          ++del_iterator_;
        } else if (std::is_eq(cmp_result)) {
          ++del_iterator_;
          return true;
        }

        break;
      }

      return false;
    }
  };

  template <class LabelIterator, class LabelEndIterator, class AddIterator, class DelIterator>
  class Iterator : public GenericIterator<LabelIterator, LabelEndIterator, AddIterator, DelIterator> {
   public:
    using Base = GenericIterator<LabelIterator, LabelEndIterator, AddIterator, DelIterator>;

    using Base::Base;

    using value_type = Primitives::LabelView;

    const value_type& operator*() const { return this->label_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      this->next();
      return *this;
    }

    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto result = *this;
      ++*this;
      return result;
    }
  };

  template <class LabelIterator, class LabelEndIterator, class AddIterator, class DelIterator>
  class NamesIterator : public GenericIterator<LabelIterator, LabelEndIterator, AddIterator, DelIterator> {
   public:
    using Base = GenericIterator<LabelIterator, LabelEndIterator, AddIterator, DelIterator>;

    using Base::Base;

    using value_type = Primitives::SymbolView;

    const value_type& operator*() const { return this->label_.first; }

    PROMPP_ALWAYS_INLINE NamesIterator& operator++() noexcept {
      this->next();
      return *this;
    }

    PROMPP_ALWAYS_INLINE NamesIterator operator++(int) noexcept {
      const auto result = *this;
      ++*this;
      return result;
    }
  };

  class Names {
   public:
    explicit Names(const LabelSetBuilder& label_set_builder) : label_set_builder_(label_set_builder) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept {
      return NamesIterator<decltype(label_set_builder_.label_set_.begin()), decltype(label_set_builder_.label_set_.end()),
                           decltype(label_set_builder_.add_.begin()), decltype(label_set_builder_.del_.begin())>(label_set_builder_);
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE friend size_t hash_value(const Names& names) noexcept { return hash::hash_of_string_list(names); }

   private:
    const LabelSetBuilder& label_set_builder_;
  };

  LabelSetBuilder(const LabelSet& label_set, const AddContainer<Label>& add, const DelContainer<String>& del) : label_set_(label_set), add_(add), del_(del) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept {
    return Iterator<decltype(label_set_.begin()), decltype(label_set_.end()), decltype(add_.begin()), decltype(del_.begin())>(*this);
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE friend size_t hash_value(const LabelSetBuilder& label_set) noexcept { return hash::hash_of_label_set(label_set); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Names names() const noexcept { return Names{*this}; }

 private:
  const LabelSet& label_set_;
  const AddContainer<Label>& add_;
  const DelContainer<String>& del_;
};

struct TimeSeries {
  LabelSet label_set;
  uint64_t timestamp;
  double value;
};

struct LabelSetLimits {
  uint32_t max_label_name_length;
  uint32_t max_label_value_length;
  uint32_t max_label_count;
};

template <class TimeSeriesLabelSet>
void read_label_set(const LabelSet& go_label_set, TimeSeriesLabelSet& time_series_label_set) {
  for (auto& go_label_view : go_label_set.pairs) {
    typename TimeSeriesLabelSet::label_type label;
    auto name = std::string_view(go_label_set.data.data() + go_label_view.name.begin, go_label_view.name.length);
    label.first = name;
    auto value = std::string_view(go_label_set.data.data() + go_label_view.value.begin, go_label_view.value.length);
    label.second = value;
    time_series_label_set.add(label);
  }
}

template <class TimeSeriesLabelSet>
void read_label_set(const LabelSet& go_label_set, TimeSeriesLabelSet& time_series_label_set, LabelSetLimits& limits) {
  if (limits.max_label_count && go_label_set.pairs.size() > limits.max_label_count) {
    throw BareBones::Exception(0x18ffd63c691bdb60, "Max label count exceeded");
  }
  for (auto& go_label_view : go_label_set.pairs) {
    typename TimeSeriesLabelSet::label_type label;
    auto name = std::string_view(go_label_set.data.data() + go_label_view.name.begin, go_label_view.name.length);
    if (limits.max_label_name_length && std::size(name) > limits.max_label_name_length) {
      throw BareBones::Exception(0x91b0aa8a8eb15681, "Label name size (%zd) exceeds the maximum name size limit", std::size(name));
    }
    label.first = name;
    auto value = std::string_view(go_label_set.data.data() + go_label_view.value.begin, go_label_view.value.length);
    if (limits.max_label_value_length && std::size(value) > limits.max_label_value_length) {
      throw BareBones::Exception(0x3214247d751d903e, "Label value size (%zd) exceeds the maximum value size limit", std::size(value));
    }
    label.second = value;
    time_series_label_set.add(label);
  }
  if (time_series_label_set.size() == 0) {
    throw BareBones::Exception(0x2c52a6423c07e065, "Label set is empty");
  }
}

template <class Samples>
void read_samples(const TimeSeries& go_time_series, Samples& samples) {
  typename Samples::value_type sample;
  sample.timestamp() = go_time_series.timestamp;
  sample.value() = go_time_series.value;
  samples.push_back(sample);
}

template <class Timeseries>
void read_timeseries(const TimeSeries& go_time_series, Timeseries& time_series) {
  read_label_set(go_time_series.label_set, time_series.label_set());
  read_samples(go_time_series, time_series.samples());
}

}  // namespace PromPP::Primitives::Go

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::Primitives::Go::StringView> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::Primitives::Go::LabelView> : std::true_type {};

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::Primitives::Go::Label> : std::true_type {};
