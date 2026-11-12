#pragma once

#include <cassert>
#include <concepts>
#include <cstdint>
#include <cstdlib>
#include <cstring>
#include <ios>
#include <limits>
#include <ostream>
#include <span>
#include <spanstream>
#include <string_view>

#include <experimental/scope>

#include "exception.h"
#include "preprocess.h"

namespace BareBones {

template <typename T>
concept HiddenExceptions = requires(T x, std::ios_base::iostate e) {
  { x.exceptions() } -> std::convertible_to<std::ios_base::iostate>;
  x.exceptions(e);
};

// The stream must outlive the guard. Its setter must install the mask before throwing.
// Setup errors propagate. Cleanup preserves the stream state and suppresses restoration errors.
template <HiddenExceptions S>
[[nodiscard]] auto scoped_stream_exceptions(S& stream, std::ios_base::iostate mask) {
  const std::ios_base::iostate original_mask = stream.exceptions();
  auto guard = std::experimental::scope_exit([&stream, original_mask]() noexcept {
    try {
      stream.exceptions(original_mask);
    } catch (...) {
      // Restoration must not replace an active exception or escape cleanup.
    }
  });
  stream.exceptions(mask);
  return guard;
}

template <typename T>
concept InputStream = HiddenExceptions<T> && requires(T x, char c, char* s, size_t n) {
  { x.get() } -> std::convertible_to<uint8_t>;
  { x.eof() } -> std::convertible_to<bool>;
  x.read(s, n);
};

template <typename T>
concept OutputStream = HiddenExceptions<T> && requires(T x, char c, char* s, size_t n) {
  x.put(c);
  x.write(s, n);
};

template <typename T>
concept StreamWithReadableBytesHint = requires(const T& x) {
  { x.readable_bytes_hint() } -> std::convertible_to<size_t>;
};

// Upper bound of the bytes that can still be read from the stream: exact for a span, the remaining budget for a stream that has
// one, and no limit for any other stream. Deserializers check a size read from the stream against it before they allocate memory.
template <class S>
[[nodiscard]] PROMPP_ALWAYS_INLINE size_t readable_bytes_hint(S& in) noexcept {
  if constexpr (StreamWithReadableBytesHint<S>) {
    return in.readable_bytes_hint();
  } else if constexpr (std::derived_from<S, std::ispanstream>) {
    const auto available = in.rdbuf()->in_avail();
    return available > 0 ? static_cast<size_t>(available) : 0;
  } else {
    return std::numeric_limits<size_t>::max();
  }
}

template <class S>
void check_readable_bytes(S& in, size_t bytes, const char* what) {
  if (const auto readable = readable_bytes_hint(in); bytes > readable) [[unlikely]] {
    throw Exception(0x7a058b8b3f6499e7, "Invalid %s size got from input: %zu bytes, but only %zu bytes can be read", what, bytes, readable);
  }
}

class ShrinkedToFitOStringStream : public std::ostream {
 public:
  explicit ShrinkedToFitOStringStream() : std::ostream(&buffer_) {}

  [[nodiscard]] std::string_view view() const noexcept { return buffer_.view(); }

  template <class T>
  [[nodiscard]] std::span<T> span() const noexcept {
    return buffer_.span<T>();
  }

 private:
  class output_buffer final : public std::streambuf {
   public:
    ~output_buffer() override { std::free(memory_); }

    [[nodiscard]] std::string_view view() const noexcept { return {memory_, size_}; }

    template <class T>
    [[nodiscard]] std::span<T> span() const noexcept {
      return {reinterpret_cast<T*>(memory_), size_};
    }

   private:
    char* memory_{};
    size_t size_{};

    int_type overflow(int_type ch) override {
      xsputn(reinterpret_cast<const char_type*>(&ch), 1);
      return ch;
    }

    std::streamsize xsputn(const char_type* s, std::streamsize count) override {
      memory_ = static_cast<char*>(std::realloc(memory_, size_ + count));
      assert(memory_ != nullptr);

      std::memcpy(memory_ + size_, s, count);
      size_ += count;

      return count;
    }
  };

  output_buffer buffer_;
};

}  // namespace BareBones
