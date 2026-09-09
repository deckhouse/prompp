#include "facts/fact_store.h"

#include <cassert>
#include <utility>

namespace epgen::facts {

SourceFileId FactStore::add_source_file(std::string_view path) {
  assert(source_files_.size() < SourceFileId::kInvalidValue);
  const SourceFileId id(static_cast<uint32_t>(source_files_.size()));
  source_files_.push_back(SourceFileDecl{.path = std::string(path)});
  return id;
}

FunctionId FactStore::add_function(FunctionDecl function) {
  assert(functions_.size() < FunctionId::kInvalidValue);
  const FunctionId id(static_cast<uint32_t>(functions_.size()));
  functions_.push_back(std::move(function));
  return id;
}

const SourceFileDecl& FactStore::source_file(SourceFileId id) const {
  return !id.is_valid() || id.get() >= source_files_.size() ? invalid_source_file_ : source_files_[id.get()];
}

const FunctionDecl& FactStore::function(FunctionId id) const {
  return !id.is_valid() || id.get() >= functions_.size() ? invalid_function_ : functions_[id.get()];
}

std::span<const SourceFileDecl> FactStore::source_files() const noexcept {
  return source_files_;
}

std::span<const FunctionDecl> FactStore::functions() const noexcept {
  return functions_;
}

}  // namespace epgen::facts
