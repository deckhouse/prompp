#pragma once

#include <cassert>
#include <cstring>
#include <fstream>
#include <iostream>
#include <span>
#include <type_traits>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "bare_bones/vector.h"

namespace PromPP::Primitives::Go {

class String {
  const char* data_;
  size_t len_;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = const char;
  using const_iterator = const char*;

  explicit String() = default;

  explicit constexpr String(std::string_view value) : data_(value.data()), len_(value.size()) {}

  static PROMPP(inline) String allocate_and_copy(std::string_view value) noexcept {
    auto data = reinterpret_cast<char*>(std::malloc(value.size() + 1));
    std::memcpy(data, value.data(), value.size());
    data[value.size()] = '\0';

    return String({data, value.size()});
  }

  static PROMPP(inline) void free_str(String& str) noexcept {
    std::free(const_cast<char*>(str.data_));
    str.reset_to(nullptr, 0);
  }

  PROMPP(inline) void reset_to(const char* data, size_t len) noexcept {
    data_ = data;
    len_ = len;
  }
  PROMPP(inline) void reset_to(const std::string_view& value) noexcept { reset_to(value.data(), value.size()); }

  [[nodiscard]] PROMPP(inline) const char* data() const noexcept { return data_; }
  [[nodiscard]] PROMPP(inline) bool empty() const noexcept { return !len_; }
  [[nodiscard]] PROMPP(inline) size_t size() const noexcept { return len_; }

  [[nodiscard]] PROMPP(inline) const_iterator begin() const noexcept { return data_; }
  [[nodiscard]] PROMPP(inline) const_iterator end() const noexcept { return data_ + len_; }

  PROMPP(inline) const char& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }

  PROMPP(inline) String& operator=(const std::string_view& value) noexcept {
    reset_to(value);
    return *this;
  }

  PROMPP(inline) bool operator<(const String& other) const noexcept {
    return static_cast<std::string_view>(*this) < static_cast<std::string_view>(other);
  }

  PROMPP(inline) bool operator==(const String& other) const noexcept { return this->size() == other.size() && std::ranges::equal(*this, other); }

  PROMPP(inline) bool operator==(const std::string_view& other) const noexcept {
    return this->size() == other.size() && std::ranges::equal(*this, other);
  }

  explicit PROMPP(inline) operator std::string_view() const noexcept { return {data_, len_}; }
};

template <class T>
struct SliceControlBlock {
  using SizeType = size_t;

  T* data{};

  union {
    SizeType items_count{};
    SizeType length_;
  };

  union {
    SizeType data_size{};
    SizeType capacity_;
  };
};

template <class T>
using Slice = BareBones::MemoryBasedVector<SliceControlBlock, T, BareBones::DefaultReallocator>;

template <class T>
class SliceView {
  T* data_;
  size_t len_;
  size_t cap_;

 public:
  using iterator_category = std::contiguous_iterator_tag;
  using value_type = const T;
  using iterator = T*;
  using const_iterator = const T*;

  PROMPP(inline) SliceView() = default;

  PROMPP(inline) SliceView(T* data, size_t len, size_t cap) noexcept : data_(data), len_(len), cap_(cap) {}
  explicit PROMPP(inline) SliceView(Slice<T>& slice) : data_(slice.data()), len_(slice.size()), cap_(slice.capacity()) {}

  PROMPP(inline) void reset_to(T* data, size_t len, size_t cap) {
    data_ = data;
    len_ = len;
    cap_ = cap;
  }

  PROMPP(inline) void reset_to(std::span<T> buffer) { reset_to(buffer.data(), buffer.size(), buffer.size()); }

  PROMPP(inline) const T* data() const noexcept { return data_; }
  PROMPP(inline) T* data() noexcept { return data_; }

  [[nodiscard]] PROMPP(inline) bool empty() const noexcept { return !len_; }
  [[nodiscard]] PROMPP(inline) size_t size() const noexcept { return len_; }
  [[nodiscard]] PROMPP(inline) size_t capacity() const noexcept { return cap_; }
  [[nodiscard]] PROMPP(inline) std::span<const T> span() const noexcept { return {data_, len_}; }

  PROMPP(inline) const_iterator begin() const noexcept { return data_; }
  const_iterator end() const noexcept { return data_ + len_; }

  PROMPP(inline) iterator begin() noexcept { return data_; }
  PROMPP(inline) iterator end() noexcept { return data_ + len_; }

  PROMPP(inline) const T& operator[](uint32_t i) const {
    assert(i < len_);
    return data_[i];
  }

  PROMPP(inline) T& operator[](uint32_t i) {
    assert(i < len_);
    return data_[i];
  }
};

class BytesStream : public std::ostream {
 public:
  explicit BytesStream(Slice<char>* s) : std::ostream(&buffer_), buffer_(s) {}

  PROMPP(inline) void reserve(size_t size) noexcept { buffer_.reserve(size); }

 private:
  class output_buffer : public std::streambuf {
   public:
    explicit output_buffer(Slice<char>* s) : slice_(s) {}

    PROMPP(inline) void reserve(size_t size) noexcept { slice_->reserve(size); }

   private:
    Slice<char>* slice_;

    int_type overflow(int_type ch) override {
      slice_->push_back(ch);
      return ch;
    }

    std::streamsize xsputn(const char_type* s, std::streamsize count) override {
      slice_->push_back(s, s + count);
      return count;
    }

    pos_type seekoff(off_type, std::ios_base::seekdir, std::ios_base::openmode) override { return slice_->size(); }
  };

  output_buffer buffer_;
};

constexpr String operator""_gs(const char* value, size_t len) noexcept { return String(std::string_view(value, len)); }

}  // namespace PromPP::Primitives::Go

template <class T>
struct BareBones::IsTriviallyReallocatable<PromPP::Primitives::Go::Slice<T>> : std::true_type {};
