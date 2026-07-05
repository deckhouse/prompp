#pragma once

#include "tagged_index.h"

#include <cstdint>
#include <optional>

namespace entrypoint_codegen::facts {

using SourceFileId = TaggedIndex<struct SourceFileTag>;
using FunctionId = TaggedIndex<struct FunctionTag>;
using LayoutId = TaggedIndex<struct LayoutTag>;
using ParamId = TaggedIndex<struct ParamTag>;
using FieldId = TaggedIndex<struct FieldTag>;
using StringId = TaggedIndex<struct StringTag>;

struct ParamRange {
  ParamId begin;
  uint32_t count;
};

struct LayoutRange {
  LayoutId begin;
  uint32_t count;
};

struct FieldRange {
  FieldId begin;
  uint32_t count;
};

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

enum class Severity : uint8_t {
  kInfo,
  kWarning,
  kError,
};

enum class DiagnosticCode : uint8_t {
  kClangDiagnostic,
  kUnsupportedReturnType,
  kUnsupportedParamCount,
  kUnsupportedParamType,
  kUnknownParamRole,
  kInvalidTwoParamOrder,
  kInvalidSecondParamRole,
  kMissingArgumentsLayout,
  kMissingResultLayout,
  kUnexpectedArgumentsLayout,
  kUnexpectedResultLayout,
  kMissingNamePrefix,
  kMissingCLinkage,
  kMissingEntrypointAttribute,
  kRuntimeMemoryUsage,
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
  FieldRange fields;
  SourceLocation location;
};

struct FunctionDecl {
  StringId name;
  StringId return_type_spelling;
  StringId documentation;
  BridgeKind bridge_kind;
  ParamRange params;
  LayoutRange layouts;
  SourceLocation location;
  bool has_c_linkage;
};

struct Diagnostic {
  DiagnosticCode code;
  std::optional<StringId> message;
  Severity severity;
  std::optional<FunctionId> function;
  SourceLocation location;
};

}  // namespace entrypoint_codegen::facts