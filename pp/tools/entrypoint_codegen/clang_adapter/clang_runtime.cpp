#include "clang_adapter/clang_runtime.h"

#include <cassert>
#include <optional>
#include <span>
#include <stdexcept>
#include <string_view>
#include <unordered_map>
#include <utility>

namespace epgen::clang_adapter {

namespace {

CursorKind cursor_kind_for(CXCursorKind kind) {
  switch (kind) {
    case CXCursor_FunctionDecl: {
      return CursorKind::kFunctionDecl;
    }
    case CXCursor_AnnotateAttr: {
      return CursorKind::kAnnotateAttr;
    }
    case CXCursor_FieldDecl: {
      return CursorKind::kFieldDecl;
    }
    case CXCursor_StructDecl: {
      return CursorKind::kStructDecl;
    }
    case CXCursor_TypeAliasDecl: {
      return CursorKind::kTypeAliasDecl;
    }
    default: {
      return CursorKind::kOther;
    }
  }
}

CXChildVisitResult child_visit_result_for(VisitResult result) {
  switch (result) {
    case VisitResult::kBreak: {
      return CXChildVisit_Break;
    }
    case VisitResult::kContinue: {
      return CXChildVisit_Continue;
    }
    case VisitResult::kRecurse: {
      return CXChildVisit_Recurse;
    }
  }
  return CXChildVisit_Continue;
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

std::string clang_string_to_string(CXString value) {
  const char* raw = clang_getCString(value);
  std::string out = raw == nullptr ? std::string() : std::string(raw);
  clang_disposeString(value);
  return out;
}

std::string path_for_file(CXFile file) {
  if (file == nullptr) {
    return {};
  }
  return normalize_path(clang_string_to_string(clang_getFileName(file)));
}

std::string diagnostic_message(CXDiagnostic diagnostic) {
  return clang_string_to_string(clang_formatDiagnostic(diagnostic, clang_defaultDiagnosticDisplayOptions()));
}

diagnostics::Severity diagnostic_severity_for(CXDiagnosticSeverity severity) {
  if (severity == CXDiagnostic_Warning) {
    return diagnostics::Severity::kWarning;
  }
  if (severity == CXDiagnostic_Error || severity == CXDiagnostic_Fatal) {
    return diagnostics::Severity::kError;
  }
  return diagnostics::Severity::kInfo;
}

enum class SourceFileOrigin : uint8_t {
  kInput,
  kDiscovered,
};

struct SourceFile {
  CXFile file = nullptr;
  facts::SourceFileId id;
  SourceFileOrigin origin = SourceFileOrigin::kDiscovered;
};

}  // namespace

std::string normalize_path(std::string path) {
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

std::string TypeView::spelling() const {
  return clang_string_to_string(clang_getTypeSpelling(type_));
}

CursorView TypeView::canonical_declaration() const {
  return CursorView(clang_getTypeDeclaration(clang_getCanonicalType(type_)));
}

std::string CursorView::spelling() const {
  return clang_string_to_string(clang_getCursorSpelling(cursor_));
}

std::string CursorView::raw_comment() const {
  return clang_string_to_string(clang_Cursor_getRawCommentText(cursor_));
}

TypeView CursorView::type() const {
  return TypeView(clang_getCursorType(cursor_));
}

TypeView CursorView::result_type() const {
  return TypeView(clang_getResultType(clang_getCursorType(cursor_)));
}

CursorKind CursorView::kind() const {
  return cursor_kind_for(clang_getCursorKind(cursor_));
}

bool CursorView::is_null() const {
  return clang_Cursor_isNull(cursor_);
}

bool CursorView::is_definition() const {
  return clang_isCursorDefinition(cursor_);
}

bool CursorView::has_c_language() const {
  return clang_getCursorLanguage(cursor_) == CXLanguage_C;
}

int CursorView::argument_count() const {
  return clang_Cursor_getNumArguments(cursor_);
}

CursorView CursorView::argument(int index) const {
  return CursorView(clang_Cursor_getArgument(cursor_, index));
}

void visit_children(CursorView cursor, VisitResult (*visitor)(CursorView cursor, CursorView parent, void* data), void* data) {
  std::pair<VisitResult (*)(CursorView, CursorView, void*), void*> state{visitor, data};
  clang_visitChildren(
      cursor.cursor_,
      [](CXCursor cursor, CXCursor parent, CXClientData data) {
        auto& state = *static_cast<std::pair<VisitResult (*)(CursorView, CursorView, void*), void*>*>(data);
        return child_visit_result_for(state.first(CursorView(cursor), CursorView(parent), state.second));
      },
      &state);
}

class ParseSession::Impl {
 public:
  Impl(ParseSession& owner, const ParseOptions& options, const std::filesystem::path& source_file)
      : owner_(owner),
        source_files_(options.memory_resource),
        source_file_by_handle_(options.memory_resource),
        source_file_by_path_(options.memory_resource),
        args_(options.memory_resource) {
    if (index_.get() == nullptr) {
      throw std::runtime_error("failed to create libclang index");
    }

    register_input_file(source_file);
    parse_translation_unit(options, source_file);
  }

  Impl(ParseSession& owner,
       const ParseOptions& options,
       std::span<const std::filesystem::path> source_files,
       std::string_view virtual_source_path,
       std::string_view virtual_source)
      : owner_(owner),
        source_files_(options.memory_resource),
        source_file_by_handle_(options.memory_resource),
        source_file_by_path_(options.memory_resource),
        args_(options.memory_resource) {
    if (index_.get() == nullptr) {
      throw std::runtime_error("failed to create libclang index");
    }

    register_input_files(source_files);
    parse_translation_unit(options, virtual_source_path, virtual_source);
  }

  [[nodiscard]] CXTranslationUnit translation_unit() const { return translation_unit_.get(); }

  void add_clang_diagnostics() {
    const unsigned count = clang_getNumDiagnostics(translation_unit_.get());
    for (unsigned i = 0; i < count; ++i) {
      CXDiagnostic diagnostic = clang_getDiagnostic(translation_unit_.get(), i);

      owner_.diagnostics_.add(diagnostics::Diagnostic{
          .code = diagnostics::DiagnosticCode::kClangDiagnostic,
          .message = diagnostic_message(diagnostic),
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

  [[nodiscard]] bool is_input_source_file(CursorView cursor) {
    CXFile file = nullptr;
    unsigned line = 0;
    unsigned column = 0;
    unsigned offset = 0;

    clang_getSpellingLocation(clang_getCursorLocation(cursor.cursor_), &file, &line, &column, &offset);

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
  void register_input_file(const std::filesystem::path& source_file) {
    const std::string path = normalize_path(std::filesystem::absolute(source_file).lexically_normal().string());
    register_source_file(path, SourceFileOrigin::kInput, nullptr);
  }

  void register_input_files(std::span<const std::filesystem::path> source_files) {
    source_files_.reserve(source_files.size());
    source_file_by_path_.reserve(source_files.size());

    for (const std::filesystem::path& file : source_files) {
      register_input_file(file);
    }
  }

  void parse_translation_unit(const ParseOptions& options, const std::filesystem::path& source_file) {
    args_.reserve(options.clang_args.size());
    for (const std::string& arg : options.clang_args) {
      args_.push_back(arg.c_str());
    }

    const std::string source_path = std::filesystem::absolute(source_file).lexically_normal().string();
    CXTranslationUnit raw_tu = nullptr;
    const CXErrorCode parse_result = clang_parseTranslationUnit2(index_.get(), source_path.c_str(), args_.data(), static_cast<int>(args_.size()), nullptr, 0,
                                                                 CXTranslationUnit_KeepGoing, &raw_tu);
    if (parse_result != CXError_Success || raw_tu == nullptr) {
      throw std::runtime_error("failed to parse libclang translation unit");
    }
    translation_unit_.reset(raw_tu);
  }

  void parse_translation_unit(const ParseOptions& options, std::string_view virtual_source_path, std::string_view virtual_source) {
    args_.reserve(options.clang_args.size());
    for (const std::string& arg : options.clang_args) {
      args_.push_back(arg.c_str());
    }

    const std::string source_path(virtual_source_path);
    CXUnsavedFile unsaved_file{
        .Filename = source_path.c_str(),
        .Contents = virtual_source.data(),
        .Length = static_cast<unsigned long>(virtual_source.size()),
    };

    CXTranslationUnit raw_tu = nullptr;
    const CXErrorCode parse_result = clang_parseTranslationUnit2(index_.get(), unsaved_file.Filename, args_.data(), static_cast<int>(args_.size()),
                                                                 &unsaved_file, 1, CXTranslationUnit_KeepGoing, &raw_tu);
    if (parse_result != CXError_Success || raw_tu == nullptr) {
      throw std::runtime_error("failed to parse aggregate libclang translation unit");
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

    return register_source_file(path, SourceFileOrigin::kDiscovered, file)->id;
  }

  [[nodiscard]] SourceFile* find_source_file_by_handle(CXFile file) {
    if (file == nullptr) {
      return nullptr;
    }
    const auto it = source_file_by_handle_.find(file);
    if (it == source_file_by_handle_.end()) {
      return nullptr;
    }
    return &source_files_[it->second];
  }

  [[nodiscard]] SourceFile* find_source_file_by_path(CXFile file, std::string_view path) {
    if (path.empty()) {
      return nullptr;
    }

    const std::pmr::string key(path, owner_.memory_resource());
    const auto it = source_file_by_path_.find(key);
    if (it == source_file_by_path_.end()) {
      return nullptr;
    }
    SourceFile& source_file = source_files_[it->second];
    if (file != nullptr && source_file.file == nullptr) {
      source_file.file = file;
      source_file_by_handle_.emplace(file, it->second);
    }
    return &source_file;
  }

  SourceFile* register_source_file(std::string_view path, SourceFileOrigin origin, CXFile file) {
    if (SourceFile* existing = find_source_file_by_path(file, path); existing != nullptr) {
      if (origin == SourceFileOrigin::kInput) {
        existing->origin = SourceFileOrigin::kInput;
      }
      return existing;
    }

    const facts::SourceFileId id = owner_.facts().add_source_file(path);
    const size_t index = source_files_.size();
    source_files_.push_back(SourceFile{
        .file = file,
        .id = id,
        .origin = origin,
    });
    source_file_by_path_.emplace(std::pmr::string(path, owner_.memory_resource()), index);
    if (file != nullptr) {
      source_file_by_handle_.emplace(file, index);
    }
    return &source_files_.back();
  }

  ParseSession& owner_;
  std::pmr::vector<SourceFile> source_files_;
  std::pmr::unordered_map<CXFile, size_t> source_file_by_handle_;
  std::pmr::unordered_map<std::pmr::string, size_t> source_file_by_path_;
  std::pmr::vector<const char*> args_;
  ClangIndexWrapper index_;
  ClangTranslationUnitWrapper translation_unit_;
};

ParseSession::ParseSession(const ParseOptions& options,
                           diagnostics::DiagnosticSet& diagnostic_set,
                           facts::FactArena& facts,
                           const std::filesystem::path& source_file)
    : memory_resource_(options.memory_resource), diagnostics_(diagnostic_set), facts_(facts) {
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  impl_ = allocator.allocate(1);
  try {
    allocator.construct(impl_, *this, options, source_file);
  } catch (...) {
    allocator.deallocate(impl_, 1);
    impl_ = nullptr;
    throw;
  }
}

ParseSession::ParseSession(const ParseOptions& options,
                           diagnostics::DiagnosticSet& diagnostic_set,
                           facts::FactArena& facts,
                           std::span<const std::filesystem::path> source_files,
                           std::string_view virtual_source_path,
                           std::string_view virtual_source)
    : memory_resource_(options.memory_resource), diagnostics_(diagnostic_set), facts_(facts) {
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  impl_ = allocator.allocate(1);
  try {
    allocator.construct(impl_, *this, options, source_files, virtual_source_path, virtual_source);
  } catch (...) {
    allocator.deallocate(impl_, 1);
    impl_ = nullptr;
    throw;
  }
}

ParseSession::~ParseSession() {
  if (impl_ == nullptr) {
    return;
  }
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  allocator.destroy(impl_);
  allocator.deallocate(impl_, 1);
}

CursorView ParseSession::root_cursor() const {
  return CursorView(clang_getTranslationUnitCursor(impl_->translation_unit()));
}

void ParseSession::add_clang_diagnostics() {
  impl_->add_clang_diagnostics();
}

facts::SourceLocation ParseSession::source_location_for(CursorView cursor) {
  return impl_->source_location_for(clang_getCursorLocation(cursor.cursor_));
}

bool ParseSession::is_input_source_file(CursorView cursor) {
  return impl_->is_input_source_file(cursor);
}

}  // namespace epgen::clang_adapter
