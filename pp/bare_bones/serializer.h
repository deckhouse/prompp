#pragma once

#include <cstdint>
#include <fstream>
#include <limits>
#include <span>

#include <experimental/scope>

#include "exception.h"
#include "streams.h"

namespace BareBones {

inline constinit uint8_t kSerializerVersion = 1;

template <class T>
void serialize(std::ostream& stream, std::span<T> items) {
  const auto exception_guard = scoped_stream_exceptions(stream, std::ifstream::failbit | std::ifstream::badbit);

  stream.put(static_cast<char>(kSerializerVersion));

  const auto count = static_cast<uint32_t>(items.size());
  stream.write(reinterpret_cast<const char*>(&count), sizeof(count));

  if (count > 0) [[likely]] {
    stream.write(reinterpret_cast<const char*>(items.data()), sizeof(T) * count);
  }
}

template <class Vector>
void deserialize(std::istream& stream, Vector& vector) {
  auto vector_size = static_cast<uint32_t>(vector.size());
  auto sg1 = std::experimental::scope_fail([vector_size, &vector] { vector.resize(vector_size); });

  if (const uint8_t version = stream.get(); stream.eof()) {
    return;
  } else if (version != kSerializerVersion) {
    throw Exception(0xed3acf7e0d0327d2, "Invalid serializer version %d got from input stream, only version %d is supported", version, kSerializerVersion);
  }

  const auto exception_guard = scoped_stream_exceptions(stream, std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

  uint32_t count{0};
  stream.read(reinterpret_cast<char*>(&count), sizeof(count));

  if (count > 0) [[likely]] {
    vector.resize(vector_size + count);
    static_assert(std::numeric_limits<std::streamsize>::max() / sizeof(uint32_t) >= std::numeric_limits<uint32_t>::max());
    const auto byte_count = static_cast<std::streamsize>(sizeof(uint32_t)) * count;
    stream.read(reinterpret_cast<char*>(&vector[vector_size]), byte_count);
  }
}

}  // namespace BareBones
