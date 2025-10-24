#pragma once

#include <string_view>

#include "primitives/label_set.h"

namespace metrics {

class Metric {
 public:
  using LabelSet = PromPP::Primitives::LabelSet;

  Metric() = delete;
  explicit Metric(std::string_view name) : name_(name) {}
  Metric(const Metric&) = delete;
  Metric(Metric&&) noexcept = delete;
  Metric& operator=(const Metric&) = delete;
  Metric& operator=(Metric&&) noexcept = delete;
  virtual ~Metric() = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view name() const noexcept { return name_; }

  virtual void serialize(const LabelSet& labels, std::string& buffer) const = 0;

  [[nodiscard]] virtual size_t object_size() const noexcept = 0;

 private:
  std::string_view name_;
};

}  // namespace metrics