#pragma once

#include <functional>
#include <memory>
#include <string_view>

#include "series_index/symbol_source.h"

namespace series_index {

template <class ValueType>
class PostShrinkSnapshotResolver {
 public:
  using ForEachSymbolIdCallback = std::function<void(uint32_t name_id, uint32_t value_id)>;

  virtual ~PostShrinkSnapshotResolver() = default;

  [[nodiscard]] virtual std::unique_ptr<PostShrinkSnapshotResolver<ValueType>> clone() const = 0;
  [[nodiscard]] virtual ValueType at(uint32_t id) const = 0;
  [[nodiscard]] virtual std::string_view symbol(uint32_t name_id, uint32_t value_id) const = 0;
  virtual void for_each_symbol_id(const ForEachSymbolIdCallback& callback) const = 0;
};

template <class Snapshot, class ValueType>
class TypedPostShrinkSnapshotResolver final : public PostShrinkSnapshotResolver<ValueType> {
 public:
  explicit TypedPostShrinkSnapshotResolver(const Snapshot& snapshot) : snapshot_(&snapshot) {}

  [[nodiscard]] std::unique_ptr<PostShrinkSnapshotResolver<ValueType>> clone() const override {
    return std::make_unique<TypedPostShrinkSnapshotResolver<Snapshot, ValueType>>(*snapshot_);
  }

  [[nodiscard]] ValueType at(uint32_t id) const override { return (*snapshot_)[id]; }

  [[nodiscard]] std::string_view symbol(uint32_t name_id, uint32_t value_id) const override {
    const auto view = snapshot_->data_view();
    return value_id == kKeyOnlyValueId ? view.key_symbol(name_id) : view.value_symbol(name_id, value_id);
  }

  void for_each_symbol_id(const typename PostShrinkSnapshotResolver<ValueType>::ForEachSymbolIdCallback& callback) const override {
    const auto view = snapshot_->data_view();
    for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
      callback(it.key_id(), it.value_id());
    }
  }

 private:
  const Snapshot* snapshot_;
};

}  // namespace series_index
