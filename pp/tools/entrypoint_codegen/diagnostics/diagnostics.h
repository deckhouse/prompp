#pragma once

#include "facts/facts.h"

#include <cstdint>
#include <memory_resource>
#include <optional>
#include <span>
#include <string>
#include <vector>

namespace entrypoint_codegen::diagnostics {

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

struct Diagnostic {
  DiagnosticCode code;
  std::optional<std::string> message;
  Severity severity;
  std::optional<facts::FunctionId> function;
  std::optional<facts::SourceLocation> location;
};

class DiagnosticSet {
 public:
  explicit DiagnosticSet(std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource());

  void add(Diagnostic diagnostic);

  [[nodiscard]] std::span<const Diagnostic> diagnostics() const noexcept;
  [[nodiscard]] uint32_t count() const noexcept;
  [[nodiscard]] bool empty() const noexcept;

 private:
  std::pmr::vector<Diagnostic> diagnostics_;
};

}  // namespace entrypoint_codegen::diagnostics
