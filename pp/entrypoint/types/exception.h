#pragma once

#include <exception>
#include <ostream>
#include <source_location>
#include <utility>

#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"

namespace entrypoint::types {

// Temporary policy for entrypoints whose ABI has no error result.
// Exceptions must not unwind through Go frames. Define a shared error policy before adding recovery here.
template <class Function>
PROMPP_ALWAYS_INLINE void invoke_without_error_result(Function&& function) noexcept {
  try {
    std::forward<Function>(function)();
  } catch (...) {
    std::terminate();
  }
}

PROMPP_ALWAYS_INLINE void handle_current_exception(std::ostream& out, const std::source_location location = std::source_location::current()) {
  out << location.function_name() << ": ";

  try {
    std::rethrow_exception(std::current_exception());
  } catch (const BareBones::Exception& e) {
    out << e.message() << '\n' << e.stacktrace();
  } catch (const std::exception& e) {
    out << "caught a std::exception, what: " << e.what();
  } catch (...) {
    out << "caught an unknown exception";
  }
}

}  // namespace entrypoint::types
