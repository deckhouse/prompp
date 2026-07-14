#pragma once

#include "bare_bones/preprocess.h"

#define PROMPP_DETAIL_entrypoint(kind) PROMPP_DETAIL_entrypoint_##kind()

#ifdef __clang__
#define PROMPP_DETAIL_entrypoint_cgo() __attribute__((annotate("prompp.entrypoint.cgo")))
#define PROMPP_DETAIL_entrypoint_fastcgo() __attribute__((annotate("prompp.entrypoint.fastcgo")))
#else
#define PROMPP_DETAIL_entrypoint_cgo()
#define PROMPP_DETAIL_entrypoint_fastcgo()
#endif
