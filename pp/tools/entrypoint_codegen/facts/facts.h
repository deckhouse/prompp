#pragma once

#include "tagged_index.h"

#include <cstdint>

namespace epgen::facts {

using SourceFileId = TaggedIndex<struct SourceFileTag>;
using FunctionId = TaggedIndex<struct FunctionTag>;
using LayoutId = TaggedIndex<struct LayoutTag>;
using ParamId = TaggedIndex<struct ParamTag>;
using FieldId = TaggedIndex<struct FieldTag>;
using StringId = TaggedIndex<struct StringTag>;

using ParamListId = TaggedIndex<struct ParamListTag>;
using LayoutListId = TaggedIndex<struct LayoutListTag>;
using FieldListId = TaggedIndex<struct FieldListTag>;

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
  StringId path;

  [[nodiscard]] bool is_valid() const noexcept { return path.is_valid(); }
};

struct ParamDecl {
  StringId name;
  StringId type_spelling;
  ParamRole role = ParamRole::kOther;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return name.is_valid() && type_spelling.is_valid() && location.is_valid(); }
};

struct FieldDecl {
  StringId name;
  StringId type_spelling;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return name.is_valid() && type_spelling.is_valid() && location.is_valid(); }
};

struct LayoutDecl {
  LayoutKind kind = LayoutKind::kArguments;
  FieldListId fields;
  SourceLocation location;

  [[nodiscard]] bool is_valid() const noexcept { return fields.is_valid() && location.is_valid(); }
};

struct FunctionDecl {
  StringId name;
  StringId return_type_spelling;
  StringId documentation;
  BridgeKind bridge_kind = BridgeKind::kUnknown;
  ParamListId params;
  LayoutListId layouts;
  SourceLocation location;
  bool has_c_linkage = false;

  [[nodiscard]] bool is_valid() const noexcept {
    return name.is_valid() && return_type_spelling.is_valid() && documentation.is_valid() && params.is_valid() && layouts.is_valid() && location.is_valid();
  }
};

}  // namespace epgen::facts
