#include "emit/json.h"

#include "diagnostics/diagnostic_catalog.h"

#include <ostream>
#include <stdexcept>
#include <string>
#include <string_view>

namespace entrypoint_codegen::emit {

namespace {

std::string json_escape(std::string_view input) {
  constexpr char kHexDigits[] = "0123456789abcdef";

  std::string out;
  out.reserve(input.size() + 8);
  for (const unsigned char ch : input) {
    switch (ch) {
      case '\\': {
        out += "\\\\";
        break;
      }
      case '"': {
        out += "\\\"";
        break;
      }
      case '\n': {
        out += "\\n";
        break;
      }
      case '\r': {
        out += "\\r";
        break;
      }
      case '\t': {
        out += "\\t";
        break;
      }
      default: {
        if (ch < 0x20) {
          out += "\\u00";
          out.push_back(kHexDigits[ch >> 4]);
          out.push_back(kHexDigits[ch & 0x0F]);
          break;
        }
        out.push_back(static_cast<char>(ch));
      }
    }
  }
  return out;
}

std::string_view bridge_kind_name(facts::BridgeKind kind) {
  switch (kind) {
    case facts::BridgeKind::kUnknown: {
      return "unknown";
    }
    case facts::BridgeKind::kCGo: {
      return "cgo";
    }
    case facts::BridgeKind::kFastCGo: {
      return "fastcgo";
    }
  }
  return "unknown";
}

std::string_view param_role_name(facts::ParamRole role) {
  switch (role) {
    case facts::ParamRole::kArgs: {
      return "args";
    }
    case facts::ParamRole::kRes: {
      return "res";
    }
    case facts::ParamRole::kOther: {
      return "other";
    }
  }
  return "other";
}

std::string_view layout_kind_name(facts::LayoutKind kind) {
  switch (kind) {
    case facts::LayoutKind::kArguments: {
      return "arguments";
    }
    case facts::LayoutKind::kResult: {
      return "result";
    }
  }
  return "unknown";
}

void write_location(std::ostream& out, const facts::EntrypointFacts& facts, facts::SourceLocation location) {
  out << "{"
      << "\"file\": \"" << json_escape(facts.string(facts.source_file(location.file).path)) << "\", "
      << "\"line\": " << location.line << ", "
      << "\"column\": " << location.column << "}";
}

void write_params(std::ostream& out, const facts::EntrypointFacts& facts, facts::ParamRange range) {
  out << "[";
  const auto params = facts.params(range);
  for (size_t i = 0; i < params.size(); ++i) {
    if (i != 0) {
      out << ", ";
    }
    const facts::ParamDecl& param = params[i];
    out << "{"
        << "\"name\": \"" << json_escape(facts.string(param.name)) << "\", "
        << "\"type\": \"" << json_escape(facts.string(param.type_spelling)) << "\", "
        << "\"role\": \"" << param_role_name(param.role) << "\", "
        << "\"location\": ";
    write_location(out, facts, param.location);
    out << "}";
  }
  out << "]";
}

void write_layouts(std::ostream& out, const facts::EntrypointFacts& facts, facts::LayoutRange range) {
  out << "[";
  const auto layouts = facts.layouts(range);
  for (size_t i = 0; i < layouts.size(); ++i) {
    if (i != 0) {
      out << ", ";
    }
    const facts::LayoutDecl& layout = layouts[i];
    out << "{"
        << "\"kind\": \"" << layout_kind_name(layout.kind) << "\", "
        << "\"location\": ";
    write_location(out, facts, layout.location);
    out << ", \"fields\": [";
    const auto fields = facts.fields(layout.fields);
    for (size_t field_index = 0; field_index < fields.size(); ++field_index) {
      if (field_index != 0) {
        out << ", ";
      }
      const facts::FieldDecl& field = fields[field_index];
      out << "{"
          << "\"name\": \"" << json_escape(facts.string(field.name)) << "\", "
          << "\"type\": \"" << json_escape(facts.string(field.type_spelling)) << "\", "
          << "\"location\": ";
      write_location(out, facts, field.location);
      out << "}";
    }
    out << "]}";
  }
  out << "]";
}

void write_source_files(std::ostream& out, const facts::EntrypointFacts& facts) {
  out << "  \"source_files\": [\n";
  const auto source_files = facts.source_files();
  for (size_t i = 0; i < source_files.size(); ++i) {
    out << "    {\"path\": \"" << json_escape(facts.string(source_files[i].path)) << "\"}";
    if (i + 1 != source_files.size()) {
      out << ",";
    }
    out << "\n";
  }
  out << "  ]";
}

void write_function(std::ostream& out, const facts::EntrypointFacts& facts, const facts::FunctionDecl& function) {
  out << "    {\n";
  out << "      \"name\": \"" << json_escape(facts.string(function.name)) << "\",\n";
  out << "      \"return_type\": \"" << json_escape(facts.string(function.return_type_spelling)) << "\",\n";
  out << "      \"bridge_kind\": \"" << bridge_kind_name(function.bridge_kind) << "\",\n";
  out << "      \"has_c_linkage\": " << (function.has_c_linkage ? "true" : "false") << ",\n";
  out << "      \"documentation\": \"" << json_escape(facts.string(function.documentation)) << "\",\n";
  out << "      \"location\": ";
  write_location(out, facts, function.location);
  out << ",\n";
  out << "      \"params\": ";
  write_params(out, facts, function.params);
  out << ",\n";
  out << "      \"layouts\": ";
  write_layouts(out, facts, function.layouts);
  out << "\n";
  out << "    }";
}

void write_functions(std::ostream& out, const facts::EntrypointFacts& facts) {
  out << "  \"functions\": [\n";
  const auto functions = facts.functions();
  for (size_t i = 0; i < functions.size(); ++i) {
    write_function(out, facts, functions[i]);
    if (i + 1 != functions.size()) {
      out << ",";
    }
    out << "\n";
  }
  out << "  ]";
}

void write_diagnostic_function(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::Diagnostic& diagnostic) {
  if (diagnostic.function.has_value()) {
    out << "\"" << json_escape(facts.string(facts.function(*diagnostic.function).name)) << "\"";
    return;
  }
  out << "null";
}

void write_diagnostic(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::Diagnostic& diagnostic) {
  const std::string_view message = diagnostics::diagnostic_message(diagnostic);
  out << "    {"
      << "\"code\": \"" << json_escape(diagnostics::diagnostic_code_name(diagnostic.code)) << "\", "
      << "\"message\": \"" << json_escape(message) << "\", "
      << "\"severity\": \"" << diagnostics::severity_name(diagnostic.severity) << "\", "
      << "\"function\": ";
  write_diagnostic_function(out, facts, diagnostic);
  out << ", \"location\": ";
  if (diagnostic.location.has_value()) {
    write_location(out, facts, *diagnostic.location);
  } else {
    out << "null";
  }
  out << "}";
}

void write_diagnostics(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  out << "  \"diagnostics\": [\n";
  const auto diagnostic_values = diagnostic_set.diagnostics();
  for (size_t i = 0; i < diagnostic_values.size(); ++i) {
    write_diagnostic(out, facts, diagnostic_values[i]);
    if (i + 1 != diagnostic_values.size()) {
      out << ",";
    }
    out << "\n";
  }
  out << "  ]";
}

}  // namespace

void write_json(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  if (!out) {
    throw std::runtime_error("output stream is not writable");
  }

  out << "{\n";
  write_source_files(out, facts);
  out << ",\n";
  write_functions(out, facts);
  out << ",\n";
  write_diagnostics(out, facts, diagnostic_set);
  out << "\n";
  out << "}\n";
}

}  // namespace entrypoint_codegen::emit
