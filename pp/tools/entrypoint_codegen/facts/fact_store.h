#pragma once

#include "facts/facts.h"

#include <span>
#include <string_view>
#include <vector>

namespace epgen::facts {

class FactStore {
 public:
  FactStore() = default;

  [[nodiscard]] SourceFileId add_source_file(std::string_view path);
  FunctionId add_function(FunctionDecl function);

  [[nodiscard]] const SourceFileDecl& source_file(SourceFileId id) const;
  [[nodiscard]] const FunctionDecl& function(FunctionId id) const;
  [[nodiscard]] std::span<const SourceFileDecl> source_files() const noexcept;
  [[nodiscard]] std::span<const FunctionDecl> functions() const noexcept;

 private:
  SourceFileDecl invalid_source_file_{.path = std::string(kInvalidValuePlaceholder)};
  FunctionDecl invalid_function_;
  std::vector<SourceFileDecl> source_files_;
  std::vector<FunctionDecl> functions_;
};

}  // namespace epgen::facts
