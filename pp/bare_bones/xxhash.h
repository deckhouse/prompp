#pragma once

#include <string>

#include "bare_bones/compiler.h"

#define XXH_INLINE_ALL
PRAGMA_DIAGNOSTIC(push)
PRAGMA_DIAGNOSTIC(ignored DIAGNOSTIC_MAYBE_UNINITIALIZED)
#include <xxHash/xxhash.h>
PRAGMA_DIAGNOSTIC(pop)

namespace BareBones {

class XXHash3 {
 public:
  template <class Number>
    requires std::is_arithmetic_v<Number>
  PROMPP_ALWAYS_INLINE void extend(Number value) noexcept {
    // if value here is a constant, compiler break the code with optimisations
    compiler::do_not_optimize(value);
    extend(&value, sizeof(value));
  }
  PROMPP_ALWAYS_INLINE void extend(const void* buffer, size_t size) noexcept { hash_ = XXH3_64bits_withSeed(buffer, size, hash_); }
  PROMPP_ALWAYS_INLINE void extend(std::string_view buffer) noexcept { extend(buffer.data(), buffer.size()); }
  PROMPP_ALWAYS_INLINE void extend(std::string_view label_name, std::string_view label_value) noexcept {
    hash_ = XXH3_64bits_withSeed(label_name.data(), label_name.size(), hash_) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), hash_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t hash() const noexcept { return hash_; }
  [[nodiscard]] explicit PROMPP_ALWAYS_INLINE operator uint64_t() const noexcept { return hash_; }

  auto operator<=>(const XXHash3& other) const noexcept = default;

  XXHash3& operator=(uint64_t hash) noexcept {
    hash_ = hash;
    return *this;
  }

  PROMPP_ALWAYS_INLINE static uint64_t hash(const char* data, size_t size) noexcept {
    XXHash3 hasher;
    hasher.extend(data, size);
    return hasher.hash();
  }

  PROMPP_ALWAYS_INLINE static uint64_t hash(const std::string_view& str) noexcept { return hash(str.data(), str.size()); }
  PROMPP_ALWAYS_INLINE static uint64_t hash(const std::string& str) noexcept { return hash(str.data(), str.size()); }

 private:
  uint64_t hash_{};
};

class XXHash {
 public:
  PROMPP_ALWAYS_INLINE XXHash() { XXH64_reset(&state_, 0); }

  PROMPP_ALWAYS_INLINE void extend(std::string_view data) noexcept { XXH64_update(&state_, data.data(), data.length()); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t digest() const noexcept { return XXH64_digest(&state_); }

 private:
  XXH64_state_t state_;
};

}  // namespace BareBones
