#pragma once

#include "bare_bones/compiler.h"  // IWYU pragma: export

// PROMPP(entrypoint, kind): mark a CGo or FastCgo definition for the libclang scanner.
#define PROMPP_DETAIL_entrypoint(kind) PROMPP_DETAIL_ENTRYPOINT_##kind()

#if PROMPP(compiler, clang)
#define PROMPP_DETAIL_ENTRYPOINT_cgo() __attribute__((annotate("prompp.entrypoint.cgo")))
#define PROMPP_DETAIL_ENTRYPOINT_fastcgo() __attribute__((annotate("prompp.entrypoint.fastcgo")))
#else
#define PROMPP_DETAIL_ENTRYPOINT_cgo()
#define PROMPP_DETAIL_ENTRYPOINT_fastcgo()
#endif
