#pragma once

#include "macro.h"  // IWYU pragma: export

// PROMPP(arch, name): query the compilation target using a literal architecture name below.
#define PROMPP_DETAIL_arch(name) PROMPP_DETAIL_ARCH_##name()

// PROMPP(arch, amd64): 1 for the 64-bit x86 target, 0 otherwise; also valid in #if.
#if defined(__x86_64__) || defined(_M_AMD64)
#  define PROMPP_DETAIL_ARCH_amd64() 1
#else
#  define PROMPP_DETAIL_ARCH_amd64() 0
#endif

// PROMPP(arch, arm64): 1 for the AArch64 target, 0 otherwise; also valid in #if.
#if defined(__aarch64__) || defined(_M_ARM64)
#  define PROMPP_DETAIL_ARCH_arm64() 1
#else
#  define PROMPP_DETAIL_ARCH_arm64() 0
#endif

// PROMPP(isa, name): query instructions enabled for this compilation, not runtime CPU support.
#define PROMPP_DETAIL_isa(name) PROMPP_DETAIL_ISA_##name()

// PROMPP(isa, x86_bmi): 1 when x86 BMI instructions are enabled, 0 otherwise.
#ifdef __BMI__
#  define PROMPP_DETAIL_ISA_x86_bmi() 1
#else
#  define PROMPP_DETAIL_ISA_x86_bmi() 0
#endif

// PROMPP(isa, x86_sse42): 1 when x86 SSE4.2 instructions are enabled, 0 otherwise.
#ifdef __SSE4_2__
#  define PROMPP_DETAIL_ISA_x86_sse42() 1
#else
#  define PROMPP_DETAIL_ISA_x86_sse42() 0
#endif

// PROMPP(isa, arm_crc32): 1 when Arm CRC32 instructions are enabled, 0 otherwise.
#ifdef __ARM_FEATURE_CRC32
#  define PROMPP_DETAIL_ISA_arm_crc32() 1
#else
#  define PROMPP_DETAIL_ISA_arm_crc32() 0
#endif
