#include "common.h"
#include "annotations.hpp"
#include "bare_bones/jemalloc.h"

#if !JEMALLOC_AVAILABLE
#  include <malloc.h>
#endif

#include "primitives/go_slice.h"

/**
 * @brief Free memory allocated for response as []byte
 *
 * @param args *[]byte
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_free_bytes(void* args) {
  using Slice = PromPP::Primitives::Go::Slice<char>;
  using Arguments = Slice;

  static_cast<Arguments*>(args)->~Slice();
}

extern "C" void je_jemalloc_constructor(void);

/**
 * @brief Initialize jemalloc for the selected CPU implementation.
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_jemalloc_init() {
#if JEMALLOC_AVAILABLE
  je_jemalloc_constructor();
#endif
}

/**
 * @brief Return allocator memory statistics.
 *
 * @param res {
 *   in_use    int64 // active bytes
 *   allocated int64 // allocated bytes
 *   resident  int64 // resident bytes
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_mem_info(void* res) {
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

/**
 * @brief Dump jemalloc memory profile to file
 *
 * @param args {
 *     filename string
 * }
 * @param res {
 *     error int32
 * }
 */
extern "C" PROMPP(entrypoint, cgo) void prompp_dump_memory_profile([[maybe_unused]] void* args, void* res) {
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
