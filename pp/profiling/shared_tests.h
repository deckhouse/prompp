#pragma once

#include "profiling.h"

namespace PromPP::Profiling::Testing {

inline void shared_site() {
  PROMPP_PROFILE(scope, "shared test site");
}

void shared_site_from_another_translation_unit();

}  // namespace PromPP::Profiling::Testing
