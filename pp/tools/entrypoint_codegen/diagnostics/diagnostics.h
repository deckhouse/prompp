#pragma once

#include "facts/facts.h"

#include <cstdint>
#include <memory_resource>
#include <optional>
#include <span>
#include <string>
#include <string_view>
#include <vector>

namespace epgen::diagnostics {

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

struct SeverityCounts {
  uint32_t total = 0;
  uint32_t errors = 0;
  uint32_t warnings = 0;
  uint32_t infos = 0;

  [[nodiscard]] bool has_errors() const noexcept;
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

[[nodiscard]] SeverityCounts count_by_severity(const DiagnosticSet& diagnostic_set);
[[nodiscard]] std::string_view diagnostic_code_name(DiagnosticCode code);
[[nodiscard]] std::string_view diagnostic_default_message(DiagnosticCode code);
[[nodiscard]] std::string_view diagnostic_message(const Diagnostic& diagnostic);
[[nodiscard]] std::string_view severity_name(Severity severity);

}  // namespace epgen::diagnostics
