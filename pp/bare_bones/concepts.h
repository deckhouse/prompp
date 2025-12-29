#pragma once

#include <chrono>

namespace BareBones::concepts {

template <class T>
concept is_dereferenceable = requires(T t) {
  { *t };
};

template <class T>
concept has_allocated_memory = requires(const T& t) {
  { t.allocated_memory() } -> std::convertible_to<size_t>;
};

template <class T>
concept has_allocated_memory_field = requires(const T& t) {
  { t.allocated_memory } -> std::convertible_to<size_t>;
};

template <class T>
concept has_earliest_timestamp_field = requires(const T& t) {
  { t.earliest_timestamp } -> std::convertible_to<int64_t>;
};

template <class T>
concept has_latest_timestamp_field = requires(const T& t) {
  { t.latest_timestamp } -> std::convertible_to<int64_t>;
};

template <class T>
concept has_series_field = requires(const T& t) {
  { t.series } -> std::convertible_to<uint32_t>;
};

template <class T>
concept has_remainder_size_field = requires(const T& t) {
  { t.remainder_size } -> std::convertible_to<uint32_t>;
};

template <class T>
concept dereferenceable_has_allocated_memory = requires(const T& t) {
  { t->allocated_memory() } -> std::convertible_to<size_t>;
};

template <class T>
concept has_capacity = requires(const T& t) {
  { t.capacity() } -> std::convertible_to<size_t>;
};

template <class T>
concept has_size = requires(const T& t) {
  { t.size() } -> std::convertible_to<size_t>;
};

template <class T>
concept has_reserve = requires(T& r, uint32_t n) {
  { r.reserve(n) } -> std::same_as<void>;
};

template <class Clock>
concept SystemClockInterface = requires(Clock& clock) {
  { typename Clock::time_point{} } -> std::same_as<std::chrono::system_clock::time_point>;
  { clock.now() };
};

template <class Clock>
concept SteadyClockInterface = requires(Clock& clock) {
  { typename Clock::time_point{} } -> std::same_as<std::chrono::steady_clock::time_point>;
  { clock.now() };
};

template <class T>
concept arithmetic = std::integral<T> || std::floating_point<T>;

}  // namespace BareBones::concepts
