#pragma once

#include "facts/facts.h"

#include <cstdint>
#include <memory_resource>
#include <span>
#include <string_view>

namespace entrypoint_codegen::facts {

class EntrypointFacts {
 public:
  explicit EntrypointFacts(std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource());
  ~EntrypointFacts();

  EntrypointFacts(EntrypointFacts&&) noexcept;
  EntrypointFacts& operator=(EntrypointFacts&&) noexcept;

  EntrypointFacts(const EntrypointFacts&) = delete;
  EntrypointFacts& operator=(const EntrypointFacts&) = delete;

  StringId add_string(std::string_view value);
  SourceFileId add_source_file(std::string_view path);
  ParamRange add_params(std::span<const ParamDecl> params);
  FieldRange add_fields(std::span<const FieldDecl> fields);
  LayoutRange add_layouts(std::span<const LayoutDecl> layouts);
  FunctionId add_function(FunctionDecl function);

  [[nodiscard]] std::string_view string(StringId id) const;

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const;
  [[nodiscard]] const FunctionDecl& function(FunctionId id) const;

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const;
  [[nodiscard]] std::span<const FunctionDecl> functions() const;

  [[nodiscard]] std::span<const ParamDecl> params(FunctionId id) const;
  [[nodiscard]] std::span<const ParamDecl> params(ParamRange range) const;

  [[nodiscard]] std::span<const LayoutDecl> layouts(FunctionId id) const;
  [[nodiscard]] std::span<const LayoutDecl> layouts(LayoutRange range) const;

  [[nodiscard]] std::span<const FieldDecl> fields(LayoutId id) const;
  [[nodiscard]] std::span<const FieldDecl> fields(FieldRange range) const;

 private:
  class Impl;
  Impl* impl_ = nullptr;
  std::pmr::memory_resource* memory_resource_ = std::pmr::get_default_resource();
};

}  // namespace entrypoint_codegen::facts
