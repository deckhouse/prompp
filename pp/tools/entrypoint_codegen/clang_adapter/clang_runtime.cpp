#include "clang_adapter/clang_runtime.h"

#include <clang-c/Index.h>

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

bool should_report_clang_diagnostic(CXDiagnosticSeverity severity) {
  return severity == CXDiagnostic_Error || severity == CXDiagnostic_Fatal;
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

class AstContext {
 public:
  AstContext() = default;

  CursorView make_cursor(CXCursor cursor) {
    const auto index = static_cast<uint32_t>(cursors_.size());
    cursors_.push_back(cursor);
    return CursorView(this, index);
  }

  TypeView make_type(CXType type) {
    const auto index = static_cast<uint32_t>(types_.size());
    types_.push_back(type);
    return TypeView(this, index);
  }

  [[nodiscard]] CXCursor cx_cursor(CursorView cursor) const {
    assert(cursor.context_ == this);
    assert(cursor.index_ < cursors_.size());
    return cursors_[cursor.index_];
  }

  [[nodiscard]] CXType cx_type(TypeView type) const {
    assert(type.context_ == this);
    assert(type.index_ < types_.size());
    return types_[type.index_];
  }

 private:
  std::vector<CXCursor> cursors_;
  std::vector<CXType> types_;
};

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
  return clang_string_to_string(clang_getTypeSpelling(context_->cx_type(*this)));
}

CursorView TypeView::canonical_declaration() const {
  return context_->make_cursor(clang_getTypeDeclaration(clang_getCanonicalType(context_->cx_type(*this))));
}

std::string CursorView::spelling() const {
  return clang_string_to_string(clang_getCursorSpelling(context_->cx_cursor(*this)));
}

std::string CursorView::raw_comment() const {
  return clang_string_to_string(clang_Cursor_getRawCommentText(context_->cx_cursor(*this)));
}

TypeView CursorView::type() const {
  return context_->make_type(clang_getCursorType(context_->cx_cursor(*this)));
}

TypeView CursorView::result_type() const {
  return context_->make_type(clang_getResultType(clang_getCursorType(context_->cx_cursor(*this))));
}

CursorKind CursorView::kind() const {
  return cursor_kind_for(clang_getCursorKind(context_->cx_cursor(*this)));
}

bool CursorView::is_null() const {
  return clang_Cursor_isNull(context_->cx_cursor(*this));
}

bool CursorView::is_definition() const {
  return clang_isCursorDefinition(context_->cx_cursor(*this));
}

bool CursorView::has_c_language() const {
  return clang_getCursorLanguage(context_->cx_cursor(*this)) == CXLanguage_C;
}

int CursorView::argument_count() const {
  return clang_Cursor_getNumArguments(context_->cx_cursor(*this));
}

CursorView CursorView::argument(int index) const {
  return context_->make_cursor(clang_Cursor_getArgument(context_->cx_cursor(*this), index));
}

namespace detail {

void visit_children_impl(CursorView cursor, const ChildVisitor& visitor) {
  struct VisitState {
    AstContext& context;
    const ChildVisitor& visitor;
  } state{
      .context = *cursor.context_,
      .visitor = visitor,
  };

  clang_visitChildren(
      cursor.context_->cx_cursor(cursor),
      [](CXCursor cursor, CXCursor parent, CXClientData data) {
        auto& state = *static_cast<VisitState*>(data);
        return child_visit_result_for(state.visitor(state.context.make_cursor(cursor), state.context.make_cursor(parent)));
      },
      &state);
}

}  // namespace detail

class ParseSession::Impl : public AstContext {
 public:
  Impl(ParseSession& owner, const ParseOptions& options, VirtualParseInput input)
      : AstContext(), owner_(owner), source_files_(), source_file_by_handle_(), source_file_by_path_(), args_() {
    if (index_.get() == nullptr) {
      throw std::runtime_error("failed to create libclang index");
    }

    register_input_files(input.source_files);
    parse_translation_unit(options, input.virtual_source_path, input.virtual_source);
  }

  [[nodiscard]] CXTranslationUnit translation_unit() const { return translation_unit_.get(); }

  void add_clang_diagnostics() {
    const unsigned count = clang_getNumDiagnostics(translation_unit_.get());
    for (unsigned i = 0; i < count; ++i) {
      CXDiagnostic diagnostic = clang_getDiagnostic(translation_unit_.get(), i);
      const CXDiagnosticSeverity severity = clang_getDiagnosticSeverity(diagnostic);
      if (!should_report_clang_diagnostic(severity)) {
        clang_disposeDiagnostic(diagnostic);
        continue;
      }

      owner_.diagnostics_.add(diagnostics::Diagnostic{
          .code = diagnostics::DiagnosticCode::kClangDiagnostic,
          .message = diagnostic_message(diagnostic),
          .severity = diagnostic_severity_for(severity),
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

    clang_getSpellingLocation(clang_getCursorLocation(cx_cursor(cursor)), &file, &line, &column, &offset);

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
      path = std::string(facts::kInvalidValuePlaceholder);
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

    const std::string key(path);
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
    source_file_by_path_.emplace(path, index);
    if (file != nullptr) {
      source_file_by_handle_.emplace(file, index);
    }
    return &source_files_.back();
  }

  ParseSession& owner_;
  std::vector<SourceFile> source_files_;
  std::unordered_map<CXFile, size_t> source_file_by_handle_;
  std::unordered_map<std::string, size_t> source_file_by_path_;
  std::vector<const char*> args_;
  ClangIndexWrapper index_;
  ClangTranslationUnitWrapper translation_unit_;
};

ParseSession::ParseSession(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set, facts::FactStore& facts, VirtualParseInput input)
    : diagnostics_(diagnostic_set), facts_(facts) {
  impl_ = new Impl(*this, options, input);
}

ParseSession::~ParseSession() {
  if (impl_ == nullptr) {
    return;
  }
  delete impl_;
}

CursorView ParseSession::root_cursor() const {
  return impl_->make_cursor(clang_getTranslationUnitCursor(impl_->translation_unit()));
}

void ParseSession::add_clang_diagnostics() {
  impl_->add_clang_diagnostics();
}

facts::SourceLocation ParseSession::source_location_for(CursorView cursor) {
  return impl_->source_location_for(clang_getCursorLocation(impl_->cx_cursor(cursor)));
}

bool ParseSession::is_input_source_file(CursorView cursor) {
  return impl_->is_input_source_file(cursor);
}

}  // namespace epgen::clang_adapter
