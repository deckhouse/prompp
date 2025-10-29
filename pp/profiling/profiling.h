#pragma once

#ifdef PROMPP_PROFILING_ENABLE
#define TRACY_ENABLE
#endif

// callstack
#ifndef PROMPP_PROFILING_CALLSTACK
#define PROMPP_PROFILING_CALLSTACK 0
#endif
#define TRACY_CALLSTACK PROMPP_PROFILING_CALLSTACK

#include "tracy/Tracy.hpp"