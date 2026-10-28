#pragma once

#include <cstddef>
#include <cstdint>
#include <source_location>

#include "bare_bones/macro.h"

// PROMPP_PROFILE(operation, ...): profile scopes with Bazel --config=profile; otherwise, arguments are not evaluated.
// Each marker records the caller's file, line and function. PROMPP_PROFILE_DETAIL_* macros are private.
#define PROMPP_PROFILE(operation, ...) PROMPP(cat, PROMPP_PROFILE_DETAIL_, operation)(__VA_ARGS__)

// PROMPP_PROFILE(enabled): 1 when profiling is enabled, 0 otherwise; also valid in #if.
#if defined(PROMPP_PROFILING_ENABLE) && PROMPP_PROFILING_ENABLE
#  define PROMPP_PROFILE_DETAIL_enabled() 1
#else
#  define PROMPP_PROFILE_DETAIL_enabled() 0
#endif

namespace PromPP::Profiling {

class ScopedZone;

// Keep each Marker and its name alive until process exit.
class Marker final {
 public:
  explicit Marker(const char* name, std::source_location location = std::source_location::current()) noexcept;
  Marker(const Marker&) = delete;
  Marker(Marker&&) = delete;
  Marker& operator=(const Marker&) = delete;
  Marker& operator=(Marker&&) = delete;

  [[nodiscard]] ScopedZone enter() const noexcept;

 private:
  friend class ScopedZone;
#if PROMPP_PROFILE(enabled)
  struct Impl;
  const Impl& impl() const noexcept;

  // profiling.cpp validates the implementation against this inline capacity.
  static constexpr std::size_t kStorageSize = 32;
  static constexpr std::size_t kStorageAlignment = alignof(void*);
  alignas(kStorageAlignment) std::byte storage_[kStorageSize];
#endif
};

// Zones must end in reverse construction order on their starting thread.
class [[nodiscard]] ScopedZone final {
 public:
  explicit ScopedZone(const Marker& marker) noexcept;
  ~ScopedZone() noexcept;
  ScopedZone(const ScopedZone&) = delete;
  ScopedZone(ScopedZone&&) = delete;
  ScopedZone& operator=(const ScopedZone&) = delete;
  ScopedZone& operator=(ScopedZone&&) = delete;

  // This zone must be the innermost active zone when its value is set.
  void set_value(uint64_t value) noexcept;

 private:
#if PROMPP_PROFILE(enabled)
  struct Impl;
  Impl& impl() noexcept;

  // Debug storage also holds the nesting link. Release storage stays byte-aligned.
#  if !PROMPP(debug)
  static constexpr std::size_t kStorageSize = 1;
  static constexpr std::size_t kStorageAlignment = alignof(std::byte);
#  else
  static constexpr std::size_t kStorageSize = 2 * sizeof(void*);
  static constexpr std::size_t kStorageAlignment = alignof(void*);
#  endif
  alignas(kStorageAlignment) std::byte storage_[kStorageSize];
#endif
};

#if !PROMPP_PROFILE(enabled)
inline Marker::Marker(const char*, std::source_location) noexcept {}
inline ScopedZone Marker::enter() const noexcept { return ScopedZone{*this}; }
inline ScopedZone::ScopedZone(const Marker&) noexcept {}
inline ScopedZone::~ScopedZone() noexcept = default;
inline void ScopedZone::set_value(uint64_t) noexcept {}
#endif

}  // namespace PromPP::Profiling

#if PROMPP_PROFILE(enabled)
// Pass the caller location into the lambda so its function name does not become operator().
// Keep explicit macro alignment. The formatter models PROMPP for declaration layout.
// clang-format off
#  define PROMPP_PROFILE_DETAIL_MAKE(zone, label)                                              \
  [[maybe_unused]] ::PromPP::Profiling::ScopedZone zone{                                       \
    [](const char* name, std::source_location location) -> const ::PromPP::Profiling::Marker& { \
      static const ::PromPP::Profiling::Marker marker{name, location};                         \
      return marker;                                                                           \
    }(label, std::source_location::current())                                                  \
  }
// clang-format on

// PROMPP_PROFILE(function); Measure until this C++ scope ends, using the current function name.
// Put anonymous function and scope markers on separate lines at the same lexical level.
#  define PROMPP_PROFILE_DETAIL_function() PROMPP_PROFILE_DETAIL_MAKE(PROMPP(cat, prompp_profile_zone_, __LINE__), __func__)
// PROMPP_PROFILE(scope, "decode"); Measure until this scope ends; use an ordinary string literal label.
#  define PROMPP_PROFILE_DETAIL_scope(name) PROMPP_PROFILE_DETAIL_MAKE(PROMPP(cat, prompp_profile_zone_, __LINE__), "" name)
// PROMPP_PROFILE(named_scope, zone, "decode"); Name a scope guard for PROMPP_PROFILE(value, zone, count).
// Use an ordinary string literal label and a distinct guard name in this scope.
#  define PROMPP_PROFILE_DETAIL_named_scope(zone, name) PROMPP_PROFILE_DETAIL_MAKE(zone, "" name)
// PROMPP_PROFILE(value, zone, count); Set a uint64_t value on the innermost active zone, after its children end.
// clang-format off
#  define PROMPP_PROFILE_DETAIL_value(zone, ...)                                               \
  do {                                                                                         \
    (zone).set_value((__VA_ARGS__));                                                           \
  } while (false)
// clang-format on
#else
#  define PROMPP_PROFILE_DETAIL_function() ((void)0)
#  define PROMPP_PROFILE_DETAIL_scope(name) ((void)0)
#  define PROMPP_PROFILE_DETAIL_named_scope(zone, name) ((void)0)
#  define PROMPP_PROFILE_DETAIL_value(zone, ...) ((void)0)
#endif
