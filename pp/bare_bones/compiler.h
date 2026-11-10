#pragma once

#include "macro.h"  // IWYU pragma: export
#include "preprocess.h"

// PROMPP(compiler, name): query gcc or clang, including in #if. Clang does not count as GCC.
#define PROMPP_DETAIL_compiler(name) PROMPP_DETAIL_COMPILER_##name()
#if defined(__clang__)
#define PROMPP_DETAIL_COMPILER_clang() 1
#define PROMPP_DETAIL_COMPILER_gcc() 0
#elif defined(__GNUC__)
#define PROMPP_DETAIL_COMPILER_clang() 0
#define PROMPP_DETAIL_COMPILER_gcc() 1
#else
#error Compiler diagnostics are not supported
#endif

namespace BareBones::compiler {

template <class Tp>
static PROMPP_ALWAYS_INLINE void do_not_optimize(const Tp& value) {
  __asm__ __volatile__("" ::"m"(value));
}

};  // namespace BareBones::compiler
