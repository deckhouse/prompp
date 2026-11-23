#pragma once

#include <version>

#include "compiler.h"  // IWYU pragma: export

// PROMPP(feature, name): query library/header availability using a literal name below; also valid in #if.
#define PROMPP_DETAIL_feature(name) PROMPP_DETAIL_FEATURE_##name()

// PROMPP(test): 1 if GTest headers are available, 0 otherwise; the existing test-build heuristic, also valid in #if.
#if __has_include(<gtest/gtest.h>)
#  define PROMPP_DETAIL_test() 1
#else
#  define PROMPP_DETAIL_test() 0
#endif

// PROMPP(feature, jemalloc): 1 if jemalloc headers are available, 0 otherwise.
#if __has_include(<jemalloc/jemalloc.h>)
#  define PROMPP_DETAIL_FEATURE_jemalloc() 1
#else
#  define PROMPP_DETAIL_FEATURE_jemalloc() 0
#endif

// PROMPP(feature, std_spanstream): 1 if the spanstream header is available, 0 otherwise.
#if __has_include(<spanstream>)
#  define PROMPP_DETAIL_FEATURE_std_spanstream() 1
#else
#  define PROMPP_DETAIL_FEATURE_std_spanstream() 0
#endif

// PROMPP(feature, std_pointer_interconvertible): 1 if the standard library provides pointer-interconvertibility checks, 0 otherwise.
#ifdef __cpp_lib_is_pointer_interconvertible
#  define PROMPP_DETAIL_FEATURE_std_pointer_interconvertible() 1
#else
#  define PROMPP_DETAIL_FEATURE_std_pointer_interconvertible() 0
#endif
