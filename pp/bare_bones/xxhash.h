#pragma once

#include <span>
#include <string>

#include "bare_bones/compiler.h"

#define XXH_INLINE_ALL
PROMPP(diagnostic, push)
PROMPP(diagnostic, ignore, maybe_uninitialized)
#include <xxHash/xxhash.h>
PROMPP(diagnostic, pop)

namespace BareBones {

class XXHash3 {
 public:
  template <class Number>
    requires std::is_arithmetic_v<Number>
  PROMPP(inline) void extend(Number value) noexcept {
    // if value here is a constant, compiler break the code with optimisations
    compiler::do_not_optimize(value);
    extend(&value, sizeof(value));
  }
  PROMPP(inline) void extend(const void* buffer, size_t size) noexcept { hash_ = XXH3_64bits_withSeed(buffer, size, hash_); }
  PROMPP(inline) void extend(std::string_view buffer) noexcept { extend(buffer.data(), buffer.size()); }
  PROMPP(inline) void extend(std::string_view label_name, std::string_view label_value) noexcept {
    hash_ = XXH3_64bits_withSeed(label_name.data(), label_name.size(), hash_) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), hash_);
  }

  [[nodiscard]] PROMPP(inline) uint64_t hash() const noexcept { return hash_; }
  [[nodiscard]] explicit PROMPP(inline) operator uint64_t() const noexcept { return hash_; }

  auto operator<=>(const XXHash3& other) const noexcept = default;

  XXHash3& operator=(uint64_t hash) noexcept {
    hash_ = hash;
    return *this;
  }

  static PROMPP(inline) uint64_t hash(const void* data, size_t size) noexcept {
    XXHash3 hasher;
    hasher.extend(data, size);
    return hasher.hash();
  }
  static PROMPP(inline) uint64_t hash(const std::string_view& str) noexcept { return hash(str.data(), str.size()); }
  static PROMPP(inline) uint64_t hash(const std::string& str) noexcept { return hash(str.data(), str.size()); }
  template <class T>
  static PROMPP(inline) uint64_t hash(std::span<T> data) noexcept {
    return hash(data.data(), data.size() * sizeof(data[0]));
  }

 private:
  uint64_t hash_{};
};

class XXHash {
 public:
  PROMPP(inline) XXHash() { XXH64_reset(&state_, 0); }

  PROMPP(inline) void extend(std::string_view data) noexcept { XXH64_update(&state_, data.data(), data.length()); }
  [[nodiscard]] PROMPP(inline) uint64_t digest() const noexcept { return XXH64_digest(&state_); }

 private:
  XXH64_state_t state_;
};

}  // namespace BareBones
