#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"

namespace series_index::querier {

template <class Resolver>
concept ValueMatchResolverInterface = requires(const Resolver resolver) {
  { resolver(uint32_t()) };
};

template <class Resolver>
concept MatchResolverInterface = requires(const Resolver resolver) {
  { resolver.resolve_name(uint32_t()) };
  { resolver.value_resolver(uint32_t()) } -> ValueMatchResolverInterface;
};

class ValueMatchIdResolver {
 public:
  PROMPP_ALWAYS_INLINE static uint32_t operator()(uint32_t id) noexcept { return id; }
};

class MatchIdResolver {
 public:
  PROMPP_ALWAYS_INLINE static uint32_t resolve_name(uint32_t id) noexcept { return id; }
  PROMPP_ALWAYS_INLINE static ValueMatchIdResolver value_resolver(uint32_t) noexcept { return {}; }
};

static_assert(ValueMatchResolverInterface<ValueMatchIdResolver>);
static_assert(MatchResolverInterface<MatchIdResolver>);

}  // namespace series_index::querier