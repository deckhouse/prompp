#include "facts/fact_arena.h"

#include <cassert>
#include <memory_resource>
#include <utility>
#include <vector>

namespace epgen::facts {

class FactArena::Impl {
 public:
  explicit Impl(std::pmr::memory_resource* memory_resource)
      : invalid_source_file_(SourceFileDecl{.path = std::string(kInvalidValuePlaceholder)}), source_files_(memory_resource), functions_(memory_resource) {}

  SourceFileId add_source_file(std::string_view path) {
    assert(source_files_.size() < SourceFileId::kInvalidValue);
    const auto id = SourceFileId(static_cast<uint32_t>(source_files_.size()));
    source_files_.push_back(SourceFileDecl{.path = std::string(path)});
    return id;
  }

  FunctionId add_function(FunctionDecl function) {
    assert(functions_.size() < FunctionId::kInvalidValue);
    const auto id = FunctionId(static_cast<uint32_t>(functions_.size()));
    functions_.push_back(std::move(function));
    return id;
  }

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const {
    return !id.is_valid() || id.get() >= source_files_.size() ? invalid_source_file_ : source_files_[id.get()];
  }

  [[nodiscard]] const FunctionDecl& function(FunctionId id) const {
    return !id.is_valid() || id.get() >= functions_.size() ? invalid_function_ : functions_[id.get()];
  }

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const { return source_files_; }
  [[nodiscard]] std::span<const FunctionDecl> functions() const { return functions_; }

 private:
  SourceFileDecl invalid_source_file_;
  FunctionDecl invalid_function_;
  std::pmr::vector<SourceFileDecl> source_files_;
  std::pmr::vector<FunctionDecl> functions_;
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
  if (impl_ != nullptr) {
    std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
    allocator.destroy(impl_);
    allocator.deallocate(impl_, 1);
  }
}

FactArena::FactArena(FactArena&& other) noexcept : impl_(other.impl_), memory_resource_(other.memory_resource_) {
  other.impl_ = nullptr;
}

FactArena& FactArena::operator=(FactArena&& other) noexcept {
  if (this != &other) {
    if (impl_ != nullptr) {
      std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
      allocator.destroy(impl_);
      allocator.deallocate(impl_, 1);
    }
    impl_ = other.impl_;
    memory_resource_ = other.memory_resource_;
    other.impl_ = nullptr;
  }
  return *this;
}

std::string FactArena::add_string(std::string_view value) const {
  return std::string(value);
}
SourceFileId FactArena::add_source_file(std::string_view path) {
  return impl_->add_source_file(path);
}
std::vector<ParamDecl> FactArena::add_params(std::span<const ParamDecl> params) const {
  return {params.begin(), params.end()};
}
std::vector<FieldDecl> FactArena::add_fields(std::span<const FieldDecl> fields) const {
  return {fields.begin(), fields.end()};
}
std::vector<LayoutDecl> FactArena::add_layouts(std::span<const LayoutDecl> layouts) const {
  return {layouts.begin(), layouts.end()};
}
FunctionId FactArena::add_function(FunctionDecl function) {
  return impl_->add_function(std::move(function));
}
std::string_view FactArena::string(std::string_view value) const noexcept {
  return value;
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
  return params(function(id).params);
}
std::span<const ParamDecl> FactArena::params(const std::vector<ParamDecl>& params) const noexcept {
  return params;
}
std::span<const LayoutDecl> FactArena::layouts(FunctionId id) const {
  return layouts(function(id).layouts);
}
std::span<const LayoutDecl> FactArena::layouts(const std::vector<LayoutDecl>& layouts) const noexcept {
  return layouts;
}
std::span<const FieldDecl> FactArena::fields(const LayoutDecl& layout) const noexcept {
  return fields(layout.fields);
}
std::span<const FieldDecl> FactArena::fields(const std::vector<FieldDecl>& fields) const noexcept {
  return fields;
}

}  // namespace epgen::facts
