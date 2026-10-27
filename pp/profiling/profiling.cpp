#include "profiling.h"

#if PROMPP_PROFILE(enabled)
#include <cassert>
#include <memory>
#include <new>
#include <type_traits>

#include "client/TracyProfiler.hpp"
#include "client/TracyScoped.hpp"

namespace PromPP::Profiling {

struct Marker::Impl {
  explicit Impl(const char* name, std::source_location source) noexcept : location{name, source.function_name(), source.file_name(), source.line(), 0U} {}

  tracy::SourceLocationData location;
};

Marker::Marker(const char* name, std::source_location location) noexcept {
  static_assert(std::is_trivially_destructible_v<Impl>);
  static_assert(sizeof(Impl) <= kStorageSize);
  static_assert(kStorageAlignment % alignof(Impl) == 0);
  std::construct_at(reinterpret_cast<Impl*>(storage_), name, location);
}

const Marker::Impl& Marker::impl() const noexcept {
  return *std::launder(reinterpret_cast<const Impl*>(storage_));
}

ScopedZone Marker::enter() const noexcept {
  return ScopedZone{*this};
}

struct ScopedZone::Impl {
  explicit Impl(const tracy::SourceLocationData& location) noexcept : zone_{&location} {
#ifndef NDEBUG
    previous_zone_ = current_zone_;
    current_zone_ = this;
#endif
  }

  ~Impl() noexcept {
#ifndef NDEBUG
    assert(current_zone_ == this && "Zones must end in reverse order on their starting thread");
    current_zone_ = previous_zone_;
#endif
  }

  void set_value(uint64_t value) noexcept {
#ifndef NDEBUG
    assert(current_zone_ == this && "Values require the innermost active zone");
#endif
    zone_.Value(value);
  }

 private:
  tracy::ScopedZone zone_;
#ifndef NDEBUG
  Impl* previous_zone_;
  inline static thread_local Impl* current_zone_ = nullptr;
#endif
};

ScopedZone::ScopedZone(const Marker& marker) noexcept {
  static_assert(sizeof(Impl) <= kStorageSize);
  static_assert(kStorageAlignment % alignof(Impl) == 0);
  std::construct_at(reinterpret_cast<Impl*>(storage_), marker.impl().location);
}

ScopedZone::Impl& ScopedZone::impl() noexcept {
  return *std::launder(reinterpret_cast<Impl*>(storage_));
}

ScopedZone::~ScopedZone() noexcept {
  std::destroy_at(&impl());
}

void ScopedZone::set_value(uint64_t value) noexcept {
  impl().set_value(value);
}

}  // namespace PromPP::Profiling
#endif
