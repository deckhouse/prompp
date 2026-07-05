#include "string_table.h"

#include <cassert>
#include <cstdint>
#include <cstring>
#include <memory_resource>
#include <string_view>
#include <vector>

namespace entrypoint_codegen::facts {

class StringTable::Impl {
 public:
  explicit Impl(std::pmr::memory_resource* memory_resource) : data_(memory_resource), strings_(memory_resource) {}

  StringId add(std::string_view value) {
    assert(value.size() <= UINT32_MAX);
    assert(strings_.size() <= UINT32_MAX);

    const auto offset = static_cast<uint32_t>(data_.size());
    const auto length = static_cast<uint32_t>(value.size());
    const auto id = StringId(static_cast<uint32_t>(strings_.size()));

    data_.resize(data_.size() + value.size());
    if (!value.empty()) [[likely]] {
      std::memcpy(data_.data() + offset, value.data(), value.size());
    }
    strings_.push_back(StringData{
        .offset = offset,
        .length = length,
    });

    return id;
  }

  [[nodiscard]] std::string_view get(StringId id) const {
    const uint32_t index = id.get();
    assert(index < strings_.size());
    return string_view_for(strings_[index]);
  }

  [[nodiscard]] uint32_t size() const noexcept { return static_cast<uint32_t>(strings_.size()); }

  [[nodiscard]] bool empty() const noexcept { return strings_.empty(); }

 private:
  struct StringData {
    uint32_t offset;
    uint32_t length;
  };

  [[nodiscard]] std::string_view string_view_for(StringData string) const {
    assert(static_cast<size_t>(string.offset) + string.length <= data_.size());
    return std::string_view(reinterpret_cast<const char*>(data_.data() + string.offset), string.length);
  }

  std::pmr::vector<std::byte> data_;
  std::pmr::vector<StringData> strings_;
};

StringTable::StringTable(std::pmr::memory_resource* memory_resource) : memory_resource_(memory_resource) {
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  impl_ = allocator.allocate(1);
  allocator.construct(impl_, memory_resource_);
}

StringTable::~StringTable() {
  if (impl_ == nullptr) {
    return;
  }
  std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
  allocator.destroy(impl_);
  allocator.deallocate(impl_, 1);
}

StringTable::StringTable(StringTable&& other) noexcept : impl_(other.impl_), memory_resource_(other.memory_resource_) {
  other.impl_ = nullptr;
}

StringTable& StringTable::operator=(StringTable&& other) noexcept {
  if (this == &other) {
    return *this;
  }
  if (impl_ != nullptr) {
    std::pmr::polymorphic_allocator<Impl> allocator(memory_resource_);
    allocator.destroy(impl_);
    allocator.deallocate(impl_, 1);
  }
  impl_ = other.impl_;
  memory_resource_ = other.memory_resource_;
  other.impl_ = nullptr;
  return *this;
}

StringId StringTable::add(std::string_view value) {
  return impl_->add(value);
}

std::string_view StringTable::get(StringId id) const {
  return impl_->get(id);
}

uint32_t StringTable::size() const noexcept {
  return impl_->size();
}

bool StringTable::empty() const noexcept {
  return impl_->empty();
}

}  // namespace entrypoint_codegen::facts
