#pragma once

#include "macro.h"

// PROMPP(feature, jemalloc): 1 if jemalloc headers are available, 0 otherwise; also valid in #if.
#define PROMPP_DETAIL_feature(name) PROMPP_DETAIL_FEATURE_##name()

// PROMPP(test): 1 if GTest headers are available, 0 otherwise; the existing test-build heuristic, also valid in #if.
#if __has_include(<gtest/gtest.h>)
#  define PROMPP_DETAIL_test() 1
#else
#  define PROMPP_DETAIL_test() 0
#endif

#if __has_include(<jemalloc/jemalloc.h>)
#  define PROMPP_DETAIL_FEATURE_jemalloc() 1
#else
#  define PROMPP_DETAIL_FEATURE_jemalloc() 0
#endif
