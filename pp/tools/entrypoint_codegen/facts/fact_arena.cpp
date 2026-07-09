#include "facts/fact_arena.h"

#include "facts/string_table.h"

#include <cassert>
#include <cstdint>
#include <memory_resource>
#include <vector>

namespace epgen::facts {

namespace {

template <class Id>
struct StoredList {
  Id begin;
  uint32_t count;
};

template <class T>
std::span<const T> range_span(const std::pmr::vector<T>& values, uint32_t begin, uint32_t count) {
  const uint32_t offset = begin;
  assert(offset <= values.size());
  assert(static_cast<size_t>(offset) + count <= values.size());
  return std::span<const T>(values.data() + offset, count);
}

}  // namespace

class FactArena::Impl {
 public:
  explicit Impl(std::pmr::memory_resource* memory_resource)
      : strings_(memory_resource),
        invalid_source_file_(SourceFileDecl{.path = strings_.add(kInvalidValuePlaceholder)}),
        source_files_(memory_resource),
        functions_(memory_resource),
        params_(memory_resource),
        param_lists_(memory_resource),
        layouts_(memory_resource),
        layout_lists_(memory_resource),
        fields_(memory_resource),
        field_lists_(memory_resource) {}

  StringId add_string(std::string_view value) { return strings_.add(value); }

  SourceFileId add_source_file(std::string_view path) {
    assert(source_files_.size() < SourceFileId::kInvalidValue);
    const auto id = SourceFileId(static_cast<uint32_t>(source_files_.size()));
    source_files_.push_back(SourceFileDecl{
        .path = strings_.add(path),
    });
    return id;
  }

  ParamListId add_params(std::span<const ParamDecl> params) {
    assert(params_.size() < ParamId::kInvalidValue);
    assert(param_lists_.size() < ParamListId::kInvalidValue);
    const auto begin = ParamId(static_cast<uint32_t>(params_.size()));
    const auto id = ParamListId(static_cast<uint32_t>(param_lists_.size()));
    params_.insert(params_.end(), params.begin(), params.end());
    param_lists_.push_back(StoredList<ParamId>{
        .begin = begin,
        .count = static_cast<uint32_t>(params.size()),
    });
    return id;
  }

  FieldListId add_fields(std::span<const FieldDecl> fields) {
    assert(fields_.size() < FieldId::kInvalidValue);
    assert(field_lists_.size() < FieldListId::kInvalidValue);
    const auto begin = FieldId(static_cast<uint32_t>(fields_.size()));
    const auto id = FieldListId(static_cast<uint32_t>(field_lists_.size()));
    fields_.insert(fields_.end(), fields.begin(), fields.end());
    field_lists_.push_back(StoredList<FieldId>{
        .begin = begin,
        .count = static_cast<uint32_t>(fields.size()),
    });
    return id;
  }

  LayoutListId add_layouts(std::span<const LayoutDecl> layouts) {
    assert(layouts_.size() < LayoutId::kInvalidValue);
    assert(layout_lists_.size() < LayoutListId::kInvalidValue);
    const auto begin = LayoutId(static_cast<uint32_t>(layouts_.size()));
    const auto id = LayoutListId(static_cast<uint32_t>(layout_lists_.size()));
    layouts_.insert(layouts_.end(), layouts.begin(), layouts.end());
    layout_lists_.push_back(StoredList<LayoutId>{
        .begin = begin,
        .count = static_cast<uint32_t>(layouts.size()),
    });
    return id;
  }

  FunctionId add_function(FunctionDecl function) {
    assert(functions_.size() < FunctionId::kInvalidValue);
    const auto id = FunctionId(static_cast<uint32_t>(functions_.size()));
    functions_.push_back(function);
    return id;
  }

  [[nodiscard]] std::string_view string(StringId id) const { return strings_.get(id); }

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const {
    if (!id.is_valid() || id.get() >= source_files_.size()) {
      return invalid_source_file_;
    }
    return source_files_[id.get()];
  }

  [[nodiscard]] const FunctionDecl& function(FunctionId id) const {
    if (!id.is_valid() || id.get() >= functions_.size()) {
      return invalid_function_;
    }
    return functions_[id.get()];
  }

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const { return source_files_; }

  [[nodiscard]] std::span<const FunctionDecl> functions() const { return functions_; }

  [[nodiscard]] std::span<const ParamDecl> params(FunctionId id) const { return params(function(id).params); }

  [[nodiscard]] std::span<const ParamDecl> params(ParamListId id) const {
    if (!id.is_valid() || id.get() >= param_lists_.size()) {
      return {};
    }
    const StoredList<ParamId> list = param_lists_[id.get()];
    return range_span(params_, list.begin.get(), list.count);
  }

  [[nodiscard]] std::span<const LayoutDecl> layouts(FunctionId id) const { return layouts(function(id).layouts); }

  [[nodiscard]] std::span<const LayoutDecl> layouts(LayoutListId id) const {
    if (!id.is_valid() || id.get() >= layout_lists_.size()) {
      return {};
    }
    const StoredList<LayoutId> list = layout_lists_[id.get()];
    return range_span(layouts_, list.begin.get(), list.count);
  }

  [[nodiscard]] std::span<const FieldDecl> fields(LayoutId id) const {
    if (!id.is_valid() || id.get() >= layouts_.size()) {
      return {};
    }
    return fields(layouts_[id.get()].fields);
  }

  [[nodiscard]] std::span<const FieldDecl> fields(FieldListId id) const {
    if (!id.is_valid() || id.get() >= field_lists_.size()) {
      return {};
    }
    const StoredList<FieldId> list = field_lists_[id.get()];
    return range_span(fields_, list.begin.get(), list.count);
  }

 private:
  StringTable strings_;
  SourceFileDecl invalid_source_file_;
  FunctionDecl invalid_function_;

  std::pmr::vector<SourceFileDecl> source_files_;
  std::pmr::vector<FunctionDecl> functions_;
  std::pmr::vector<ParamDecl> params_;
  std::pmr::vector<StoredList<ParamId>> param_lists_;
  std::pmr::vector<LayoutDecl> layouts_;
  std::pmr::vector<StoredList<LayoutId>> layout_lists_;
  std::pmr::vector<FieldDecl> fields_;
  std::pmr::vector<StoredList<FieldId>> field_lists_;
};

FactArena::FactArena(std::pmr::memory_resource* memory_resource) : memory_resource_(memory_resource) {
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  impl_ = allocator.allocate(1);
  try {
    allocator.construct(impl_, memory_resource_);
  } catch (...) {
    allocator.deallocate(impl_, 1);
    impl_ = nullptr;
    throw;
  }
}

FactArena::~FactArena() {
  if (impl_ == nullptr) {
    return;
  }
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  allocator.destroy(impl_);
  allocator.deallocate(impl_, 1);
}

FactArena::FactArena(FactArena&& other) noexcept : impl_(other.impl_), memory_resource_(other.memory_resource_) {
  other.impl_ = nullptr;
}

FactArena& FactArena::operator=(FactArena&& other) noexcept {
  if (this == &other) {
    return *this;
  }
  if (impl_ != nullptr) {
    std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
    allocator.destroy(impl_);
    allocator.deallocate(impl_, 1);
  }
  impl_ = other.impl_;
  memory_resource_ = other.memory_resource_;
  other.impl_ = nullptr;
  return *this;
}

StringId FactArena::add_string(std::string_view value) {
  return impl_->add_string(value);
}

SourceFileId FactArena::add_source_file(std::string_view path) {
  return impl_->add_source_file(path);
}

ParamListId FactArena::add_params(std::span<const ParamDecl> params) {
  return impl_->add_params(params);
}

FieldListId FactArena::add_fields(std::span<const FieldDecl> fields) {
  return impl_->add_fields(fields);
}

LayoutListId FactArena::add_layouts(std::span<const LayoutDecl> layouts) {
  return impl_->add_layouts(layouts);
}

FunctionId FactArena::add_function(FunctionDecl function) {
  return impl_->add_function(function);
}

std::string_view FactArena::string(StringId id) const {
  return impl_->string(id);
}

const SourceFileDecl& FactArena::source_file(SourceFileId id) const {
  return impl_->source_file(id);
}

const FunctionDecl& FactArena::function(FunctionId id) const {
  return impl_->function(id);
}

std::span<const SourceFileDecl> FactArena::source_files() const {
  return impl_->source_files();
}

std::span<const FunctionDecl> FactArena::functions() const {
  return impl_->functions();
}

std::span<const ParamDecl> FactArena::params(FunctionId id) const {
  return impl_->params(id);
}

std::span<const ParamDecl> FactArena::params(ParamListId id) const {
  return impl_->params(id);
}

std::span<const LayoutDecl> FactArena::layouts(FunctionId id) const {
  return impl_->layouts(id);
}

std::span<const LayoutDecl> FactArena::layouts(LayoutListId id) const {
  return impl_->layouts(id);
}

std::span<const FieldDecl> FactArena::fields(LayoutId id) const {
  return impl_->fields(id);
}

std::span<const FieldDecl> FactArena::fields(FieldListId id) const {
  return impl_->fields(id);
}

}  // namespace epgen::facts
