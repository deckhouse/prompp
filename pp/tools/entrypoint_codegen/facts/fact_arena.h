#pragma once

#include "facts/facts.h"

#include <cstdint>
#include <memory_resource>
#include <span>
#include <string_view>

namespace epgen::facts {

class FactArena {
 public:
  explicit FactArena(std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource());
  ~FactArena();

  FactArena(FactArena&&) noexcept;
  FactArena& operator=(FactArena&&) noexcept;

  FactArena(const FactArena&) = delete;
  FactArena& operator=(const FactArena&) = delete;

  StringId add_string(std::string_view value);
  SourceFileId add_source_file(std::string_view path);
  ParamListId add_params(std::span<const ParamDecl> params);
  FieldListId add_fields(std::span<const FieldDecl> fields);
  LayoutListId add_layouts(std::span<const LayoutDecl> layouts);
  FunctionId add_function(FunctionDecl function);

  [[nodiscard]] std::string_view string(StringId id) const;

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const;
  [[nodiscard]] const FunctionDecl& function(FunctionId id) const;

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const;
  [[nodiscard]] std::span<const FunctionDecl> functions() const;

  [[nodiscard]] std::span<const ParamDecl> params(FunctionId id) const;
  [[nodiscard]] std::span<const ParamDecl> params(ParamListId id) const;

  [[nodiscard]] std::span<const LayoutDecl> layouts(FunctionId id) const;
  [[nodiscard]] std::span<const LayoutDecl> layouts(LayoutListId id) const;

  [[nodiscard]] std::span<const FieldDecl> fields(LayoutId id) const;
  [[nodiscard]] std::span<const FieldDecl> fields(FieldListId id) const;

 private:
  class Impl;
  Impl* impl_ = nullptr;
  std::pmr::memory_resource* memory_resource_ = std::pmr::get_default_resource();
};

}  // namespace epgen::facts
