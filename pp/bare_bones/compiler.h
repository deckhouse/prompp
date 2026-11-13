#pragma once

#include "macro.h"  // IWYU pragma: export

// PROMPP(compiler, gcc): 1 for GCC, 0 otherwise; Clang's GNU compatibility does not count.
// PROMPP(compiler, clang): 1 for Clang, 0 otherwise; both queries also work in #if.
#define PROMPP_DETAIL_compiler(name) PROMPP_DETAIL_COMPILER_##name()
#if defined(__clang__)
#  define PROMPP_DETAIL_COMPILER_clang() 1
#  define PROMPP_DETAIL_COMPILER_gcc() 0
#elif defined(__GNUC__)
#  define PROMPP_DETAIL_COMPILER_clang() 0
#  define PROMPP_DETAIL_COMPILER_gcc() 1
#else
#  error Compiler diagnostics are not supported
#endif

// PROMPP(cpp_version): the numeric __cplusplus value, for example 202302L; also valid in #if.
#define PROMPP_DETAIL_cpp_version() __cplusplus

// PROMPP(debug): 0 with NDEBUG, 1 otherwise; also valid in #if.
#ifdef NDEBUG
#  define PROMPP_DETAIL_debug() 0
// static PROMPP(inline) constexpr int f(); Adds inline; always_inline with NDEBUG, noinline otherwise.
#  define PROMPP_DETAIL_inline() inline __attribute__((always_inline))
// [](int x) PROMPP(lambda_inline) { return x; }; Same policy as PROMPP(inline), without the inline keyword.
#  define PROMPP_DETAIL_lambda_inline() __attribute__((always_inline))
#else
#  define PROMPP_DETAIL_debug() 1
#  define PROMPP_DETAIL_inline() inline __attribute__((noinline))
#  define PROMPP_DETAIL_lambda_inline() __attribute__((noinline))
#endif

// PROMPP(asan): 1 with AddressSanitizer instrumentation, 0 otherwise; also valid in #if.
#if defined(__SANITIZE_ADDRESS__)
#  define PROMPP_DETAIL_asan() 1
#elif PROMPP(compiler, clang)
#  define PROMPP_DETAIL_asan() __has_feature(address_sanitizer)
#else
#  define PROMPP_DETAIL_asan() 0
#endif

// struct PROMPP(packed) Header { ... }; Use GNU packed layout; also valid on unions and fields.
#define PROMPP_DETAIL_packed() __attribute__((__packed__))

#define PROMPP_DETAIL_STRINGIFY(value) #value

#if PROMPP(compiler, clang)
#  define PROMPP_DETAIL_DIAGNOSTIC(value) _Pragma(PROMPP_DETAIL_STRINGIFY(clang diagnostic value))
#  define PROMPP_DETAIL_IGNORE_class_memaccess() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wdynamic-class-memaccess")
#  define PROMPP_DETAIL_IGNORE_maybe_uninitialized() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wuninitialized")
// Clang has no GCC stringop-overread warning group to suppress.
#  define PROMPP_DETAIL_IGNORE_stringop_overread()
#elif PROMPP(compiler, gcc)
#  define PROMPP_DETAIL_DIAGNOSTIC(value) _Pragma(PROMPP_DETAIL_STRINGIFY(GCC diagnostic value))
#  define PROMPP_DETAIL_IGNORE_class_memaccess() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wclass-memaccess")
#  define PROMPP_DETAIL_IGNORE_maybe_uninitialized() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wmaybe-uninitialized")
#  define PROMPP_DETAIL_IGNORE_stringop_overread() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wstringop-overread")
#else
#  error Compiler diagnostics are not supported
#endif

#define PROMPP_DETAIL_IGNORE_array_bounds() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Warray-bounds")
#define PROMPP_DETAIL_IGNORE_unused_local_typedefs() PROMPP_DETAIL_DIAGNOSTIC(ignored "-Wunused-local-typedefs")

// PROMPP(diagnostic, action, ...): use push, ignore or pop below, without a semicolon.
#define PROMPP_DETAIL_diagnostic(action, ...) PROMPP_DETAIL_DIAGNOSTIC_##action(__VA_ARGS__)
// PROMPP(diagnostic, push): save warning state before a narrow suppression block.
#define PROMPP_DETAIL_DIAGNOSTIC_push() PROMPP_DETAIL_DIAGNOSTIC(push)
// PROMPP(diagnostic, pop): restore the state saved by the matching push.
#define PROMPP_DETAIL_DIAGNOSTIC_pop() PROMPP_DETAIL_DIAGNOSTIC(pop)
// PROMPP(diagnostic, ignore, array_bounds): suppress a warning until pop; explain the reason at the affected code.
// Literal names: class_memaccess, maybe_uninitialized, stringop_overread, array_bounds, unused_local_typedefs.
#define PROMPP_DETAIL_DIAGNOSTIC_ignore(warning) PROMPP_DETAIL_IGNORE_##warning()

namespace BareBones::compiler {

template <class Tp>
static PROMPP(inline) void do_not_optimize(const Tp& value) {
  __asm__ __volatile__("" ::"m"(value));
}

};  // namespace BareBones::compiler
