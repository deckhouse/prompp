#pragma once

// PROMPP(operation, ...): call a literal operation from its owning header. PROMPP_DETAIL_* macros are private.
#define PROMPP(operation, ...) PROMPP_DETAIL_##operation(__VA_ARGS__)
