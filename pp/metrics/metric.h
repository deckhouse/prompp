#pragma once

#include "third_party/protozero/pbf_writer.hpp"

#include "bare_bones/preprocess.h"

namespace metrics {

class Metric {
 public:
  Metric() = delete;
  explicit Metric(std::string_view name) : name_(name) {}
  Metric(const Metric&) = delete;
  Metric(Metric&&) noexcept = delete;
  Metric& operator=(const Metric&) = delete;
  Metric& operator=(Metric&&) noexcept = delete;
  virtual ~Metric() = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view name() const noexcept { return name_; }

  virtual void serialize(protozero::pbf_writer& writer) const = 0;

  [[nodiscard]] virtual size_t object_size() const noexcept = 0;

 private:
  std::string_view name_;
};

}  // namespace metrics