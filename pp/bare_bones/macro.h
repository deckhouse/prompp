#pragma once

// PROMPP(operation, ...): call a literal operation from its owning header. PROMPP_DETAIL_* macros are private.
#define PROMPP(operation, ...) PROMPP_DETAIL_##operation(__VA_ARGS__)

// PROMPP(cat, zone_, __LINE__): expand both arguments, then join them into one valid token.
#define PROMPP_DETAIL_cat(a, b) PROMPP_DETAIL_CAT(a, b)
#define PROMPP_DETAIL_CAT(a, b) a##b
