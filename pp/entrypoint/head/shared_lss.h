#pragma once

#include "bare_bones/allocated_memory.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"

namespace entrypoint::head {

class SharedLss {
 public:
  template <class Lss>
  explicit SharedLss(const Lss& lss) : lss_(lss) {}

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c) noexcept {
    const auto id = additional_lss_.size() + lss_.size();
    additional_lss_.emplace_back(c);
    return id;
  }

  PROMPP_ALWAYS_INLINE auto operator[](uint32_t id) const noexcept {
    if (id < lss_.size()) [[likely]] {
      return LabelSet{lss_[id]};
    }

    return LabelSet{additional_lss_[id - lss_.size()]};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return BareBones::mem::allocated_memory(additional_lss_); }

 private:
  using DecodingTable = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<BareBones::SharedSpan>;

  class LabelSet {
   public:
    class IteratorSentinel {};
    class Iterator {
     public:
      using iterator_category = std::forward_iterator_tag;
      using value_type = std::pair<std::string_view, std::string_view>;
      using difference_type = std::ptrdiff_t;

      Iterator(PromPP::Primitives::LabelSet::const_iterator begin, PromPP::Primitives::LabelSet::const_iterator end)
          : iterator_(std::in_place_index<0>, begin, end) {}
      Iterator(DecodingTable::value_type::iterator begin, DecodingTable::value_type::end_iterator end) : iterator_(std::in_place_index<1>, begin, end) {}

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        std::visit([](auto& iterator_pair) { ++iterator_pair.begin; }, iterator_);
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        const auto retval = *this;
        ++(*this);
        return retval;
      }

      PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept {
        return std::visit([](auto& iterator_pair) { return iterator_pair.begin == iterator_pair.end; }, iterator_);
      }

      PROMPP_ALWAYS_INLINE value_type operator*() const noexcept {
        return std::visit([](auto& iterator_pair) { return value_type{(*iterator_pair.begin).first, (*iterator_pair.begin).second}; }, iterator_);
      }

     private:
      struct LabelSetIterators {
        PromPP::Primitives::LabelSet::const_iterator begin;
        PromPP::Primitives::LabelSet::const_iterator end;
      };

      struct DecodingTableIterators {
        DecodingTable::value_type::iterator begin;
        DecodingTable::value_type::end_iterator end;
      };

      std::variant<LabelSetIterators, DecodingTableIterators> iterator_;
    };

    explicit LabelSet(const PromPP::Primitives::LabelSet& value) : value_(std::in_place_index<0>, &value) {}
    explicit LabelSet(DecodingTable::value_type value) : value_(std::in_place_index<1>, value) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept {
      return std::visit(
          []<typename Value>(Value& value) -> uint32_t {
            if constexpr (std::is_pointer_v<Value>) {
              return value->size();
            } else {
              return value.size();
            }
          },
          value_);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept {
      return std::visit(
          []<typename Value>(Value& value) -> Iterator {
            if constexpr (std::is_pointer_v<Value>) {
              return {value->begin(), value->end()};
            } else {
              return {value.begin(), value.end()};
            }
          },
          value_);
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const LabelSet& rhs) const noexcept {
      if (value_.index() != rhs.value_.index()) {
        return false;
      }

      if (value_.index() == 0) {
        return *std::get<const PromPP::Primitives::LabelSet*>(value_) == *std::get<const PromPP::Primitives::LabelSet*>(rhs.value_);
      }

      return std::get<DecodingTable::value_type>(value_) == std::get<DecodingTable::value_type>(rhs.value_);
    }

   private:
    std::variant<const PromPP::Primitives::LabelSet*, DecodingTable::value_type> value_;
  };

  DecodingTable lss_;
  std::vector<PromPP::Primitives::LabelSet> additional_lss_;
};

}  // namespace entrypoint::head
