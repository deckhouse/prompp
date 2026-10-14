#pragma once

#include "tagged_index.h"

#include <cstdint>
#include <string>
#include <string_view>
#include <vector>

namespace epgen::facts {

inline constexpr std::string_view kInvalidValuePlaceholder = "<invalid>";

using SourceFileId = TaggedIndex<struct SourceFileTag>;
using FunctionId = TaggedIndex<struct FunctionTag>;

enum class BridgeKind : uint8_t {
  kUnknown,
  kCGo,
  kFastCGo,
};

enum class ParamRole : uint8_t {
  kArgs,
  kRes,
  kOther,
};

enum class LayoutKind : uint8_t {
  kArguments,
  kResult,
};

struct SourceLocation {
  SourceFileId file;
  uint32_t line = 0;
  uint32_t column = 0;

  [[nodiscard]] bool is_valid() const noexcept { return file.is_valid(); }
};

struct SourceFileDecl {
  std::string path;

  [[nodiscard]] bool is_valid() const noexcept { return !path.empty(); }
};

struct ParamDecl {
  std::string name;
  std::string type_spelling;
  ParamRole role = ParamRole::kOther;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return !name.empty() && !type_spelling.empty() && location.is_valid(); }
};

struct FieldDecl {
  std::string name;
  std::string type_spelling;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return !name.empty() && !type_spelling.empty() && location.is_valid(); }
};

struct LayoutDecl {
  LayoutKind kind = LayoutKind::kArguments;
  std::vector<FieldDecl> fields;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return location.is_valid(); }
};

struct FunctionDecl {
  std::string name;
  std::string return_type_spelling;
  std::string documentation;
  BridgeKind bridge_kind = BridgeKind::kUnknown;
  std::vector<ParamDecl> params;
  std::vector<LayoutDecl> layouts;
  SourceLocation location;
  bool has_c_linkage = false;

  [[nodiscard]] bool is_valid() const noexcept { return !name.empty() && !return_type_spelling.empty() && location.is_valid(); }
};

}  // namespace epgen::facts
