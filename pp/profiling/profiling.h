#pragma once

#define PROMPP_PROF(x, ...) PROMPP_PROF_IMPL_##x(__VA_ARGS__)
#define PROMPP_CAT(x, y) x##y

/*
  #if PROMPP_PROF(profiling)
  // if profiling is enabled
  #else
  // if profiling is disabled
  #endif
*/
// enabling profiling
#ifdef PROMPP_PROFILING_ENABLE
#define TRACY_ENABLE
#define PROMPP_PROF_IMPL_profiling() true
#else
#define PROMPP_PROF_IMPL_profiling() false
#endif

// callstack
#ifndef PROMPP_PROFILING_CALLSTACK
#define PROMPP_PROFILING_CALLSTACK 0
#endif
#define TRACY_CALLSTACK PROMPP_PROFILING_CALLSTACK

#include "tracy/Tracy.hpp"

// noop
#define PROMPP_PROF_IMPL_noop() TracyNoop

/*
bool foo() {
  PROMPP_PROF(scope);                       // -> ZoneScoped()
  PROMPP_PROF(scope, N, "foo");             // -> ZoneScopedN("foo")
  PROMPP_PROF(scope, C, 0xff00ff);          // -> ZoneScopedC(0xff00ff)
  PROMPP_PROF(scope, NC, "foo", 0xff00ff);  // -> ZoneScopedNC("foo", 0xff00ff)
  return true;
}
*/

// scope
#define GET_MACRO(_0, _1, _2, _3, NAME, ...) NAME

#define PROMPP_PROF_IMPL_scope(...) \
  GET_MACRO(, __VA_ARGS__, PROMPP_PROF_IMPL_scope_3, PROMPP_PROF_IMPL_scope_2, PROMPP_PROF_IMPL_scope_1, PROMPP_PROF_IMPL_scope_0)(__VA_ARGS__)

#define PROMPP_PROF_IMPL_scope_() PROMPP_PROF_IMPL_scope_0()
#define PROMPP_PROF_IMPL_scope_0() ZoneScoped

#define PROMPP_PROF_IMPL_scope_1(TYPE) PROMPP_CAT(PROMPP_PROF_IMPL_scope_, TYPE)()
#define PROMPP_PROF_IMPL_scope_2(TYPE, A) PROMPP_CAT(PROMPP_PROF_IMPL_scope_, TYPE)(A)
#define PROMPP_PROF_IMPL_scope_3(TYPE, A, B) PROMPP_CAT(PROMPP_PROF_IMPL_scope_, TYPE)(A, B)

#define PROMPP_PROF_IMPL_scope_N(name) ZoneScopedN(name)
#define PROMPP_PROF_IMPL_scope_C(color) ZoneScopedC(color)
#define PROMPP_PROF_IMPL_scope_NC(name, color) ZoneScopedNC(name, color)

// PROMPP_PROF(text, ptr, size)
// current zone text
#define PROMPP_PROF_IMPL_text(ptr, size) ZoneText(ptr, size)

// PROMPP_PROF(name, ptr, size)
// current zone name
#define PROMPP_PROF_IMPL_name(ptr, size) ZoneName(ptr, size)

// PROMPP_PROF(color, value)
// current zone color
#define PROMPP_PROF_IMPL_color(value) ZoneColor(value)

// PROMPP_PROF(value, x)
// current zone value
#define PROMPP_PROF_IMPL_value(x) ZoneValue(x)

// PROMPP_PROF(plot, name, value)
// PROMPP_PROF(plot_config, name, type, step, fill, color)
// plot
#define PROMPP_PROF_IMPL_plot(name, value) TracyPlot(name, value)
#define PROMPP_PROF_IMPL_plot_config(name, type, step, fill, color) TracyPlotConfig(name, type, step, fill, color)

// PROMPP_PROF(alloc, ptr, size)
// PROMPP_PROF(free, ptr)
// allocations
#define PROMPP_PROF_IMPL_alloc(ptr, size) TracyAlloc(ptr, size)
#define PROMPP_PROF_IMPL_free(ptr) TracyFree(ptr)

// PROMPP_PROF(message, text)
// PROMPP_PROF(log, ptr, size)
// message, log
#define PROMPP_PROF_IMPL_message(text) TracyMessageL(text)
#define PROMPP_PROF_IMPL_log(ptr, size) TracyMessage(ptr, size)