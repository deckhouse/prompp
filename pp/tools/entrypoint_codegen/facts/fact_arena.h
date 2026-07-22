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

  std::string add_string(std::string_view value) const;
  SourceFileId add_source_file(std::string_view path);
  std::vector<ParamDecl> add_params(std::span<const ParamDecl> params) const;
  std::vector<FieldDecl> add_fields(std::span<const FieldDecl> fields) const;
  std::vector<LayoutDecl> add_layouts(std::span<const LayoutDecl> layouts) const;
  FunctionId add_function(FunctionDecl function);

  [[nodiscard]] std::string_view string(std::string_view value) const noexcept;

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const;
  [[nodiscard]] const FunctionDecl& function(FunctionId id) const;

  [[nodiscard]] std::span<const SourceFileDecl> source_files() const;
  [[nodiscard]] std::span<const FunctionDecl> functions() const;

  [[nodiscard]] std::span<const ParamDecl> params(FunctionId id) const;
  [[nodiscard]] std::span<const ParamDecl> params(const std::vector<ParamDecl>& params) const noexcept;

  [[nodiscard]] std::span<const LayoutDecl> layouts(FunctionId id) const;
  [[nodiscard]] std::span<const LayoutDecl> layouts(const std::vector<LayoutDecl>& layouts) const noexcept;

  [[nodiscard]] std::span<const FieldDecl> fields(const LayoutDecl& layout) const noexcept;
  [[nodiscard]] std::span<const FieldDecl> fields(const std::vector<FieldDecl>& fields) const noexcept;

 private:
  class Impl;
  Impl* impl_ = nullptr;
  std::pmr::memory_resource* memory_resource_ = std::pmr::get_default_resource();
};

}  // namespace epgen::facts
