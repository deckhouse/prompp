#pragma once

#include "clang_adapter/parse.h"
#include "facts/entrypoint_facts.h"
#include "facts/facts.h"

#include <clang-c/Index.h>

#include <cstdint>
#include <filesystem>
#include <memory_resource>
#include <optional>
#include <span>
#include <stdexcept>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace entrypoint_codegen::clang_adapter {

inline std::string clang_string_to_std_string(CXString value) {
  const char* raw = clang_getCString(value);
  std::string out = raw == nullptr ? std::string() : std::string(raw);
  clang_disposeString(value);
  return out;
}

inline std::string normalize_path(std::string path) {
  if (path.empty()) {
    return path;
  }

  constexpr std::string_view execroot_marker = "/execroot/_main/";
  if (const size_t marker = path.find(execroot_marker); marker != std::string::npos) {
    return path.substr(marker + execroot_marker.size());
  }

  const std::filesystem::path absolute_path(path);
  if (!absolute_path.is_absolute()) {
    return path;
  }

  std::error_code error;
  const std::filesystem::path relative_path = std::filesystem::relative(absolute_path, std::filesystem::current_path(), error);
  if (error || relative_path.empty()) {
    return path;
  }

  std::string relative = relative_path.generic_string();
  if (relative == "." || relative.starts_with("../")) {
    return path;
  }
  return relative;
}

inline std::string path_for_file(CXFile file) {
  if (file == nullptr) {
    return {};
  }
  return normalize_path(clang_string_to_std_string(clang_getFileName(file)));
}

inline std::pmr::string build_synthetic_source(std::span<const std::filesystem::path> source_files, std::pmr::memory_resource* memory_resource) {
  std::pmr::string source(memory_resource);
  for (const std::filesystem::path& file : source_files) {
    source += "#include \"";
    source += file.string();
    source += "\"\n";
  }
  return source;
}

inline facts::StringId add_diagnostic_message(facts::EntrypointFacts& facts, CXDiagnostic diagnostic) {
  CXString message = clang_formatDiagnostic(diagnostic, clang_defaultDiagnosticDisplayOptions());
  const char* raw = clang_getCString(message);
  const facts::StringId id = facts.add_string(raw == nullptr ? std::string_view() : std::string_view(raw));
  clang_disposeString(message);
  return id;
}

inline facts::Severity diagnostic_severity_for(CXDiagnosticSeverity severity) {
  if (severity == CXDiagnostic_Warning) {
    return facts::Severity::kWarning;
  }
  if (severity == CXDiagnostic_Error || severity == CXDiagnostic_Fatal) {
    return facts::Severity::kError;
  }
  return facts::Severity::kInfo;
}

class ClangIndexWrapper {
 public:
  ClangIndexWrapper() : index_(clang_createIndex(0, 0)) {}
  ~ClangIndexWrapper() {
    if (index_ != nullptr) {
      clang_disposeIndex(index_);
    }
  }

  ClangIndexWrapper(const ClangIndexWrapper&) = delete;
  ClangIndexWrapper& operator=(const ClangIndexWrapper&) = delete;

  [[nodiscard]] CXIndex get() const { return index_; }

 private:
  CXIndex index_ = nullptr;
};

class ClangTranslationUnitWrapper {
 public:
  ClangTranslationUnitWrapper() = default;
  explicit ClangTranslationUnitWrapper(CXTranslationUnit translation_unit) : translation_unit_(translation_unit) {}
  ~ClangTranslationUnitWrapper() {
    if (translation_unit_ != nullptr) {
      clang_disposeTranslationUnit(translation_unit_);
    }
  }

  ClangTranslationUnitWrapper(const ClangTranslationUnitWrapper&) = delete;
  ClangTranslationUnitWrapper& operator=(const ClangTranslationUnitWrapper&) = delete;

  void reset(CXTranslationUnit translation_unit) {
    if (translation_unit_ != nullptr) {
      clang_disposeTranslationUnit(translation_unit_);
    }
    translation_unit_ = translation_unit;
  }

  [[nodiscard]] CXTranslationUnit get() const { return translation_unit_; }

 private:
  CXTranslationUnit translation_unit_ = nullptr;
};

class ParseSession {
 public:
  explicit ParseSession(const ParseOptions& options)
      : memory_resource_(options.memory_resource),
        facts_(options.memory_resource),
        source_files_(options.memory_resource),
        synthetic_source_(options.memory_resource),
        args_(options.memory_resource) {
    if (index_.get() == nullptr) {
      throw std::runtime_error("failed to create libclang index");
    }

    register_input_files(options.source_files);
    parse_translation_unit(options);
  }

  [[nodiscard]] std::pmr::memory_resource* memory_resource() const { return memory_resource_; }
  [[nodiscard]] CXTranslationUnit translation_unit() const { return translation_unit_.get(); }

  facts::EntrypointFacts& facts() { return facts_; }
  [[nodiscard]] const facts::EntrypointFacts& facts() const { return facts_; }

  [[nodiscard]] facts::EntrypointFacts take_facts() { return std::move(facts_); }

  void add_clang_diagnostics() {
    const unsigned count = clang_getNumDiagnostics(translation_unit_.get());
    for (unsigned i = 0; i < count; ++i) {
      CXDiagnostic diagnostic = clang_getDiagnostic(translation_unit_.get(), i);

      facts_.add_diagnostic(facts::Diagnostic{
          .code = facts::DiagnosticCode::kClangDiagnostic,
          .message = add_diagnostic_message(facts_, diagnostic),
          .severity = diagnostic_severity_for(clang_getDiagnosticSeverity(diagnostic)),
          .function = std::nullopt,
          .location = source_location_for(clang_getDiagnosticLocation(diagnostic)),
      });

      clang_disposeDiagnostic(diagnostic);
    }
  }

  [[nodiscard]] facts::SourceLocation source_location_for(CXSourceLocation location) {
    CXFile file = nullptr;
    unsigned line = 0;
    unsigned column = 0;
    unsigned offset = 0;

    clang_getSpellingLocation(location, &file, &line, &column, &offset);

    return facts::SourceLocation{
        .file = get_source_file(file),
        .line = line,
        .column = column,
    };
  }

  [[nodiscard]] bool is_input_source_file(CXCursor cursor) {
    CXFile file = nullptr;
    unsigned line = 0;
    unsigned column = 0;
    unsigned offset = 0;

    clang_getSpellingLocation(clang_getCursorLocation(cursor), &file, &line, &column, &offset);

    if (file == nullptr) {
      return false;
    }
    if (const SourceFile* source_file = find_source_file_by_handle(file); source_file != nullptr) {
      return source_file->origin == SourceFileOrigin::kInput;
    }

    const std::string path = path_for_file(file);
    const SourceFile* source_file = find_source_file_by_path(file, path);
    return source_file != nullptr && source_file->origin == SourceFileOrigin::kInput;
  }

 private:
  enum class SourceFileOrigin : uint8_t {
    kInput,
    kDiscovered,
  };

  struct SourceFile {
    CXFile file = nullptr;
    facts::SourceFileId id;
    SourceFileOrigin origin = SourceFileOrigin::kDiscovered;
  };

  void register_input_files(const std::vector<std::filesystem::path>& source_files) {
    source_files_.reserve(source_files.size());

    for (const std::filesystem::path& file : source_files) {
      const std::string path = normalize_path(std::filesystem::absolute(file).lexically_normal().string());
      const facts::SourceFileId id = facts_.add_source_file(path);
      source_files_.push_back(SourceFile{
          .file = nullptr,
          .id = id,
          .origin = SourceFileOrigin::kInput,
      });
    }
  }

  void parse_translation_unit(const ParseOptions& options) {
    const std::string synthetic_path = "/tmp/entrypoint_codegen_batch.cpp";
    synthetic_source_ = build_synthetic_source(options.source_files, memory_resource_);

    CXUnsavedFile unsaved_file{
        .Filename = synthetic_path.c_str(),
        .Contents = synthetic_source_.c_str(),
        .Length = static_cast<uint64_t>(synthetic_source_.size()),
    };

    args_.reserve(options.clang_args.size());
    for (const std::string& arg : options.clang_args) {
      args_.push_back(arg.c_str());
    }

    CXTranslationUnit raw_tu = nullptr;
    const CXErrorCode parse_result = clang_parseTranslationUnit2(index_.get(), synthetic_path.c_str(), args_.data(), static_cast<int>(args_.size()),
                                                                 &unsaved_file, 1, CXTranslationUnit_KeepGoing, &raw_tu);
    if (parse_result != CXError_Success || raw_tu == nullptr) {
      throw std::runtime_error("failed to parse libclang translation unit");
    }
    translation_unit_.reset(raw_tu);
  }

  [[nodiscard]] facts::SourceFileId get_source_file(CXFile file) {
    if (SourceFile* source_file = find_source_file_by_handle(file); source_file != nullptr) {
      return source_file->id;
    }

    std::string path = path_for_file(file);
    if (path.empty()) {
      path = "<unknown>";
    }
    if (SourceFile* source_file = find_source_file_by_path(file, path); source_file != nullptr) {
      return source_file->id;
    }

    const facts::SourceFileId id = facts_.add_source_file(path);
    source_files_.push_back(SourceFile{
        .file = file,
        .id = id,
        .origin = SourceFileOrigin::kDiscovered,
    });
    return id;
  }

  [[nodiscard]] SourceFile* find_source_file_by_handle(CXFile file) {
    if (file == nullptr) {
      return nullptr;
    }
    for (SourceFile& source_file : source_files_) {
      if (source_file.file == file) {
        return &source_file;
      }
    }
    return nullptr;
  }

  [[nodiscard]] SourceFile* find_source_file_by_path(CXFile file, std::string_view path) {
    if (path.empty()) {
      return nullptr;
    }

    for (SourceFile& source_file : source_files_) {
      if (facts_.string(facts_.source_file(source_file.id).path) == path) {
        if (source_file.file == nullptr) {
          source_file.file = file;
        }
        return &source_file;
      }
    }
    return nullptr;
  }

  std::pmr::memory_resource* memory_resource_;
  facts::EntrypointFacts facts_;
  std::pmr::vector<SourceFile> source_files_;
  std::pmr::string synthetic_source_;
  std::pmr::vector<const char*> args_;
  ClangIndexWrapper index_;
  ClangTranslationUnitWrapper translation_unit_;
};

}  // namespace entrypoint_codegen::clang_adapter
