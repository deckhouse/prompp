#pragma once

#include "prometheus/promql/window_function.h"
#include "series_data/decoder/decorator/window_function_iterator.h"

namespace entrypoint::series_data {

struct SelectHints {
  ::series_data::decoder::decorator::WindowFunctionParameters function_parameters;
  PromPP::Prometheus::promql::WindowFunction window_function{PromPP::Prometheus::promql::WindowFunction::kNone};
};

}  // namespace entrypoint::series_data