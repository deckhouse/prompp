#include "facts/entrypoint_facts.h"

#include "facts/string_table.h"

#include <cassert>
#include <cstdint>
#include <memory_resource>
#include <vector>

namespace entrypoint_codegen::facts {

namespace {

template <class T>
std::span<const T> range_span(const std::pmr::vector<T>& values, uint32_t begin, uint32_t count) {
  const uint32_t offset = begin;
  assert(offset <= values.size());
  assert(static_cast<size_t>(offset) + count <= values.size());
  return std::span<const T>(values.data() + offset, count);
}

}  // namespace

class EntrypointFacts::Impl {
 public:
  explicit Impl(std::pmr::memory_resource* memory_resource)
      : strings_(memory_resource),
        source_files_(memory_resource),
        functions_(memory_resource),
        params_(memory_resource),
        layouts_(memory_resource),
        fields_(memory_resource),
        diagnostics_(memory_resource) {}

  StringId add_string(std::string_view value) { return strings_.add(value); }

  SourceFileId add_source_file(std::string_view path) {
    const auto id = SourceFileId(static_cast<uint32_t>(source_files_.size()));
    source_files_.push_back(SourceFileDecl{
        .path = strings_.add(path),
    });
    return id;
  }

  ParamRange add_params(std::span<const ParamDecl> params) {
    const auto begin = ParamId(static_cast<uint32_t>(params_.size()));
    params_.insert(params_.end(), params.begin(), params.end());
    return ParamRange{
        .begin = begin,
        .count = static_cast<uint32_t>(params.size()),
    };
  }

  FieldRange add_fields(std::span<const FieldDecl> fields) {
    const auto begin = FieldId(static_cast<uint32_t>(fields_.size()));
    fields_.insert(fields_.end(), fields.begin(), fields.end());
    return FieldRange{
        .begin = begin,
        .count = static_cast<uint32_t>(fields.size()),
    };
  }

  LayoutRange add_layouts(std::span<const LayoutDecl> layouts) {
    const auto begin = LayoutId(static_cast<uint32_t>(layouts_.size()));
    layouts_.insert(layouts_.end(), layouts.begin(), layouts.end());
    return LayoutRange{
        .begin = begin,
        .count = static_cast<uint32_t>(layouts.size()),
    };
  }

  FunctionId add_function(FunctionDecl function) {
    const auto id = FunctionId(static_cast<uint32_t>(functions_.size()));
    functions_.push_back(function);
    return id;
  }

  void add_diagnostic(Diagnostic diagnostic) { diagnostics_.push_back(diagnostic); }

  [[nodiscard]] std::string_view string(StringId id) const { return strings_.get(id); }

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const {
    assert(id.get() < source_files_.size());
    return source_files_[id.get()];
  }

  [[nodiscard]] const FunctionDecl& function(FunctionId id) const {
    assert(id.get() < functions_.size());
    return functions_[id.get()];
  }

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const { return source_files_; }

  [[nodiscard]] std::span<const FunctionDecl> functions() const { return functions_; }

  [[nodiscard]] std::span<const ParamDecl> params(FunctionId id) const { return params(function(id).params); }

  [[nodiscard]] std::span<const ParamDecl> params(ParamRange range) const { return range_span(params_, range.begin.get(), range.count); }

  [[nodiscard]] std::span<const LayoutDecl> layouts(FunctionId id) const { return layouts(function(id).layouts); }

  [[nodiscard]] std::span<const LayoutDecl> layouts(LayoutRange range) const { return range_span(layouts_, range.begin.get(), range.count); }

  [[nodiscard]] std::span<const FieldDecl> fields(LayoutId id) const {
    assert(id.get() < layouts_.size());
    return fields(layouts_[id.get()].fields);
  }

  [[nodiscard]] std::span<const FieldDecl> fields(FieldRange range) const { return range_span(fields_, range.begin.get(), range.count); }

  [[nodiscard]] std::span<const Diagnostic> diagnostics() const { return diagnostics_; }

  [[nodiscard]] uint32_t diagnostic_count() const noexcept { return static_cast<uint32_t>(diagnostics_.size()); }

 private:
  StringTable strings_;

  std::pmr::vector<SourceFileDecl> source_files_;
  std::pmr::vector<FunctionDecl> functions_;
  std::pmr::vector<ParamDecl> params_;
  std::pmr::vector<LayoutDecl> layouts_;
  std::pmr::vector<FieldDecl> fields_;
  std::pmr::vector<Diagnostic> diagnostics_;
};

EntrypointFacts::EntrypointFacts(std::pmr::memory_resource* memory_resource) : memory_resource_(memory_resource) {
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  impl_ = allocator.allocate(1);
  allocator.construct(impl_, memory_resource_);
}

EntrypointFacts::~EntrypointFacts() {
  if (impl_ == nullptr) {
    return;
  }
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  allocator.destroy(impl_);
  allocator.deallocate(impl_, 1);
}

EntrypointFacts::EntrypointFacts(EntrypointFacts&& other) noexcept : impl_(other.impl_), memory_resource_(other.memory_resource_) {
  other.impl_ = nullptr;
}

EntrypointFacts& EntrypointFacts::operator=(EntrypointFacts&& other) noexcept {
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

StringId EntrypointFacts::add_string(std::string_view value) {
  return impl_->add_string(value);
}

SourceFileId EntrypointFacts::add_source_file(std::string_view path) {
  return impl_->add_source_file(path);
}

ParamRange EntrypointFacts::add_params(std::span<const ParamDecl> params) {
  return impl_->add_params(params);
}

FieldRange EntrypointFacts::add_fields(std::span<const FieldDecl> fields) {
  return impl_->add_fields(fields);
}

LayoutRange EntrypointFacts::add_layouts(std::span<const LayoutDecl> layouts) {
  return impl_->add_layouts(layouts);
}

FunctionId EntrypointFacts::add_function(FunctionDecl function) {
  return impl_->add_function(function);
}

void EntrypointFacts::add_diagnostic(Diagnostic diagnostic) {
  impl_->add_diagnostic(diagnostic);
}

std::string_view EntrypointFacts::string(StringId id) const {
  return impl_->string(id);
}

const SourceFileDecl& EntrypointFacts::source_file(SourceFileId id) const {
  return impl_->source_file(id);
}

const FunctionDecl& EntrypointFacts::function(FunctionId id) const {
  return impl_->function(id);
}

std::span<const SourceFileDecl> EntrypointFacts::source_files() const {
  return impl_->source_files();
}

std::span<const FunctionDecl> EntrypointFacts::functions() const {
  return impl_->functions();
}

std::span<const ParamDecl> EntrypointFacts::params(FunctionId id) const {
  return impl_->params(id);
}

std::span<const ParamDecl> EntrypointFacts::params(ParamRange range) const {
  return impl_->params(range);
}

std::span<const LayoutDecl> EntrypointFacts::layouts(FunctionId id) const {
  return impl_->layouts(id);
}

std::span<const LayoutDecl> EntrypointFacts::layouts(LayoutRange range) const {
  return impl_->layouts(range);
}

std::span<const FieldDecl> EntrypointFacts::fields(LayoutId id) const {
  return impl_->fields(id);
}

std::span<const FieldDecl> EntrypointFacts::fields(FieldRange range) const {
  return impl_->fields(range);
}

std::span<const Diagnostic> EntrypointFacts::diagnostics() const {
  return impl_->diagnostics();
}

uint32_t EntrypointFacts::diagnostic_count() const noexcept {
  return impl_->diagnostic_count();
}

}  // namespace entrypoint_codegen::facts
