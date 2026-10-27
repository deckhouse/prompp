#pragma once

#include <cstddef>
#include <cstdint>
#include <source_location>

// Macro payloads are not evaluated in disabled builds. Direct C++ calls retain normal argument evaluation.
#define PROMPP_PROFILE(operation, ...) PROMPP_PROFILE_DETAIL_##operation(__VA_ARGS__)

#if defined(PROMPP_PROFILING_ENABLE) && PROMPP_PROFILING_ENABLE
#define PROMPP_PROFILE_DETAIL_enabled() 1
#else
#define PROMPP_PROFILE_DETAIL_enabled() 0
#endif

namespace PromPP::Profiling {

class ScopedZone;

// Markers describe static call sites. Their names and addresses must remain valid until process exit.
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
#ifdef NDEBUG
  static constexpr std::size_t kStorageSize = 1;
  static constexpr std::size_t kStorageAlignment = alignof(std::byte);
#else
  static constexpr std::size_t kStorageSize = 2 * sizeof(void*);
  static constexpr std::size_t kStorageAlignment = alignof(void*);
#endif
  alignas(kStorageAlignment) std::byte storage_[kStorageSize];
#endif
};

#if !PROMPP_PROFILE(enabled)
inline Marker::Marker(const char*, std::source_location) noexcept {}
inline ScopedZone Marker::enter() const noexcept {
  return ScopedZone{*this};
}
inline ScopedZone::ScopedZone(const Marker&) noexcept {}
inline ScopedZone::~ScopedZone() noexcept = default;
inline void ScopedZone::set_value(uint64_t) noexcept {}
#endif

}  // namespace PromPP::Profiling

#define PROMPP_PROFILE_DETAIL_CAT_IMPL(a, b) a##b
#define PROMPP_PROFILE_DETAIL_CAT(a, b) PROMPP_PROFILE_DETAIL_CAT_IMPL(a, b)

#if PROMPP_PROFILE(enabled)
// Pass the caller location into the lambda so its function name does not become operator().
#define PROMPP_PROFILE_DETAIL_MAKE(zone, label)                                                 \
  [[maybe_unused]] ::PromPP::Profiling::ScopedZone zone {                                       \
    [](const char* name, std::source_location location) -> const ::PromPP::Profiling::Marker& { \
      static const ::PromPP::Profiling::Marker marker{name, location};                          \
      return marker;                                                                            \
    }(label, std::source_location::current())                                                   \
  }

// Anonymous markers on the same lexical level require separate source lines.
#define PROMPP_PROFILE_DETAIL_function() PROMPP_PROFILE_DETAIL_MAKE(PROMPP_PROFILE_DETAIL_CAT(prompp_profile_zone_, __LINE__), __func__)
#define PROMPP_PROFILE_DETAIL_scope(name) PROMPP_PROFILE_DETAIL_MAKE(PROMPP_PROFILE_DETAIL_CAT(prompp_profile_zone_, __LINE__), "" name)
#define PROMPP_PROFILE_DETAIL_named_scope(zone, name) PROMPP_PROFILE_DETAIL_MAKE(zone, "" name)
#define PROMPP_PROFILE_DETAIL_value(zone, ...) \
  do {                                         \
    (zone).set_value((__VA_ARGS__));           \
  } while (false)
#else
#define PROMPP_PROFILE_DETAIL_function() ((void)0)
#define PROMPP_PROFILE_DETAIL_scope(name) ((void)0)
#define PROMPP_PROFILE_DETAIL_named_scope(zone, name) ((void)0)
#define PROMPP_PROFILE_DETAIL_value(zone, ...) ((void)0)
#endif
