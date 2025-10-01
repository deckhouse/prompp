#pragma once

#include <ranges>

#include <parallel_hashmap/phmap.h>

#include "bare_bones/preprocess.h"
#include "label_set.h"
#include "primitives.h"

namespace PromPP::Primitives {

class LabelsBuilder {
  LabelViewSet building_buf_view_;
  LabelSet building_buf_;
  phmap::flat_hash_map<Symbol, Symbol> buffer_;

 public:
  // del add label name to remove from label set.
  template <class LNameType>
  PROMPP_ALWAYS_INLINE void del(const LNameType& lname) {
    buffer_.erase(lname);
  }

  // extract we extract(move) the lebel from the builder.
  PROMPP_ALWAYS_INLINE Label extract(const std::string_view& lname) {
    if (const auto it = buffer_.find(lname); it != buffer_.end()) {
      auto&& node = buffer_.extract(it);
      return {std::move(const_cast<Symbol&>(node.key())), std::move(node.mapped())};
    }

    return {};
  }

  // get returns the value for the label with the given name. Returns an empty string if the label doesn't exist.
  PROMPP_ALWAYS_INLINE std::string_view get(const std::string_view& lname) {
    if (const auto it = buffer_.find(lname); it != buffer_.end()) {
      return it->second;
    }

    return "";
  }

  // contains check the given name if exist.
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool contains(const std::string_view& lname) const noexcept {
    if (const auto it = buffer_.find(lname); it != buffer_.end()) {
      return true;
    }

    return false;
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  template <class LNameType, class LValueType>
  PROMPP_ALWAYS_INLINE void set(const LNameType& lname, const LValueType& lvalue) {
    if (lvalue.size() == 0) [[unlikely]] {
      del(lname);
      return;
    }

    if (auto it = buffer_.find(lname); it != buffer_.end()) {
      it->second = lvalue;
      return;
    }

    buffer_[lname] = lvalue;
  }

  // returns size of building labels.
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const { return buffer_.size(); }

  // returns true if ls represents an empty set of labels.
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const { return buffer_.empty(); }

  // label_view_set - returns the label_view set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const LabelViewSet& label_view_set() {
    building_buf_view_.clear();
    building_buf_view_.append_unsorted(buffer_);
    return building_buf_view_;
  }

  // label_set - returns the label set from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE const LabelSet& label_set() {
    building_buf_.clear();
    building_buf_.append_unsorted(buffer_);
    return building_buf_;
  }

  // range - calls f on each label in the builder. Don't modify LabelsBuilderStateMap inside callback
  template <class Callback>
  PROMPP_ALWAYS_INLINE void range(Callback func) const {
    for (const auto& it : buffer_) {
      if (it.second.empty()) [[unlikely]] {
        continue;
      }

      if (!func(it.first, it.second)) {
        return;
      }
    }
  }

  // reset - clears all current state for the builder.
  PROMPP_ALWAYS_INLINE void reset() {
    building_buf_view_.clear();
    building_buf_.clear();
    buffer_.clear();
  }

  // reset - clears all current state for the builder.
  template <class SomeLabelSet>
  PROMPP_ALWAYS_INLINE void reset(const SomeLabelSet& base) {
    reset();

    for (const auto& [lname, lvalue] : base) {
      if (!lvalue.empty()) [[likely]] {
        buffer_[lname] = lvalue;
      }
    }
  }
};

}  // namespace PromPP::Primitives