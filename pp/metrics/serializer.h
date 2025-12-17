#pragma once

#include "label_set.h"
#include "metric.h"

namespace metrics {

#if 0
class Serializer {
 public:
  enum Tag : int {
    kLabelSet = 1,
    kCounter = 3,
  };

  enum LabelSetTag : int {
    kName = 1,
    kValue = 2,
  };

  const std::string& serialize(const LabelSet& label_set, const Metric* metric) {
    buffer_.clear();

    protozero::pbf_writer writer(buffer_);
    label_set.set_name(metric->name());
    serialize_label_set(label_set, writer);
    metric->serialize(writer);
    return buffer_;
  }

 private:
  std::string buffer_;

  static void serialize_label_set(const LabelSet& label_set, protozero::pbf_writer& writer) {
    for (const auto& label : label_set.labels()) {
      protozero::pbf_writer label_set_writer(writer, Tag::kLabelSet);
      label_set_writer.add_string(LabelSetTag::kName, label.first);
      label_set_writer.add_string(LabelSetTag::kValue, label.second);
    }
  }
};
#endif

}  // namespace metrics