#include "common.h"
#include "bare_bones/jemalloc.h"

#if !JEMALLOC_AVAILABLE
#include <malloc.h>
#endif

#include <cstring>

#include "primitives/go_slice.h"

extern "C" void prompp_free_bytes(void* args) {
  using Slice = PromPP::Primitives::Go::Slice<char>;

  static_cast<Slice*>(args)->~Slice();
}

namespace {
#if defined(__x86_64__) || defined(_M_AMD64)
constexpr const char* kPromppFlavor = "k8";
#elif defined(__aarch64__) || defined(_M_ARM64)
constexpr const char* kPromppFlavor = "armv8-a";
#else
constexpr const char* kPromppFlavor = "unknown";
#endif
}  // namespace

extern "C" void prompp_get_flavor(void* res) {
  struct Result {
    const char* data;
    size_t len;
  };

  auto* out = static_cast<Result*>(res);
  out->data = kPromppFlavor;
  out->len = std::strlen(kPromppFlavor);
}

extern "C" void prompp_mem_info(void* res) {
  struct Result {
    int64_t in_use;
    int64_t allocated;
    int64_t resident;
  };

  const auto out = static_cast<Result*>(res);

#if JEMALLOC_AVAILABLE
  BareBones::jemalloc::refresh_stats();

  size_t size;
  size_t size_len = sizeof(size);
  mallctl("stats.active", &size, &size_len, NULL, 0);
  out->in_use = static_cast<int64_t>(size);
  mallctl("stats.allocated", &size, &size_len, NULL, 0);
  out->allocated = static_cast<int64_t>(size);
  mallctl("stats.resident", &size, &size_len, NULL, 0);
  out->resident = static_cast<int64_t>(size);
#else
  out->in_use = mallinfo2().uordblks;
  out->resident = 0;
#endif
}

extern "C" void prompp_dump_memory_profile([[maybe_unused]] void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::String filename;
  };
  struct Result {
    int error;
  };

  const auto out = static_cast<Result*>(res);

#if JEMALLOC_AVAILABLE
  auto in = static_cast<Arguments*>(args);
  std::string filename_c_string(in->filename.data(), in->filename.size());
  const char* filename = filename_c_string.c_str();

  out->error = mallctl("prof.dump", nullptr, nullptr, static_cast<void*>(&filename), sizeof(const char*));
#else
  out->error = ENODATA;
#endif
}
