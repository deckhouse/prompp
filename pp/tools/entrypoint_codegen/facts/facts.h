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
  uint32_t line;
  uint32_t column;
};

struct SourceFileDecl {
  StringId path;
};

struct ParamDecl {
  StringId name;
  StringId type_spelling;
  ParamRole role;
  SourceLocation location;
};

struct FieldDecl {
  StringId name;
  StringId type_spelling;
  SourceLocation location;
};

struct LayoutDecl {
  LayoutKind kind;
  FieldListId fields;
  SourceLocation location;
};

struct FunctionDecl {
  StringId name;
  StringId return_type_spelling;
  StringId documentation;
  BridgeKind bridge_kind;
  ParamListId params;
  LayoutListId layouts;
  SourceLocation location;
  bool has_c_linkage;
};

}  // namespace epgen::facts
