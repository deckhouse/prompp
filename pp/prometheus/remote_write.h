#pragma once

#include <string_view>

#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/basic_pbf_writer.hpp"
#include "third_party/protozero/pbf_reader.hpp"

#include "bare_bones/exception.h"
#include "primitives/histogram.h"

namespace PromPP::Prometheus::RemoteWrite {

// Use for setting the limits on pb message properties to avoid memory DoS.
// 0 means no limits.
struct PbLabelSetMemoryLimits {
  uint32_t max_label_name_length;
  uint32_t max_label_value_length;
  uint32_t max_label_names_per_timeseries;
  size_t max_timeseries_count;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool max_timeseries_count_exceeded(uint32_t count) const {
    return max_label_name_length != 0 && count > max_timeseries_count;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool max_label_name_length_exceeded(std::string_view label_name) const {
    return max_label_name_length != 0 && label_name.length() > max_label_name_length;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool max_label_value_length_exceeded(std::string_view label_value) const {
    return max_label_value_length != 0 && label_value.length() > max_label_value_length;
  }

  template <class LabelSet>
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool max_label_names_per_timeseries_exceeded(const LabelSet& label_set) const {
    return max_label_names_per_timeseries != 0 && label_set.size() > max_label_names_per_timeseries;
  }
};

struct TimeseriesProtobufHashdexRecord {
  size_t labelset_hashval;
  std::string_view timeseries_protobuf_message;

  PROMPP_ALWAYS_INLINE explicit TimeseriesProtobufHashdexRecord(size_t lshv, std::string_view& tpm) noexcept
      : labelset_hashval(lshv), timeseries_protobuf_message(tpm) {}
};

template <class Output, class Sample>
PROMPP_ALWAYS_INLINE void write_sample(protozero::basic_pbf_writer<Output>& pb, const Sample& sample) {
  protozero::basic_pbf_writer<Output> pb_sample(pb, 2);

  if (__builtin_expect(sample.value() != 0.0, true)) {
    pb_sample.add_double(1, sample.value());
  }

  if (__builtin_expect(sample.timestamp() != 0, true)) {
    pb_sample.add_int64(2, sample.timestamp());
  }
}

template <class Output>
PROMPP_ALWAYS_INLINE void write_label(protozero::basic_pbf_writer<Output>& pb, const std::string_view& label_name, const std::string_view& label_value) {
  protozero::basic_pbf_writer<Output> pb_label(pb, 1);
  pb_label.add_string(1, label_name);
  pb_label.add_string(2, label_value);
}

template <class Output, class LabelSet>
PROMPP_ALWAYS_INLINE void write_label_set(protozero::basic_pbf_writer<Output>& pb, const LabelSet& label_set) {
  for (const auto& [label_name, label_value] : label_set) {
    write_label(pb, label_name, label_value);
  }
}

template <class Output, class Timeseries>
PROMPP_ALWAYS_INLINE void write_timeseries(protozero::basic_pbf_writer<Output>& pb, const Timeseries& timeseries) {
  protozero::basic_pbf_writer<Output> pb_timeseries(pb, 1);

  write_label_set(pb_timeseries, timeseries.label_set());

  for (const auto& sample : timeseries.samples()) {
    write_sample(pb_timeseries, sample);
  }
}

template <class ProtobufReader, class Sample>
PROMPP_ALWAYS_INLINE void read_sample(ProtobufReader& pb_sample, Sample& sample) {
  uint8_t parsed = 0;

  while (pb_sample.next()) {
    switch (pb_sample.tag()) {
      case 1:  // value
        sample.value() = pb_sample.get_double();
        parsed |= 0b01;
        break;
      case 2:  // timestamp
        sample.timestamp() = pb_sample.get_int64();
        parsed |= 0b010;
        break;
      default:
        pb_sample.skip();
    }
  }

  if (__builtin_expect((parsed & 0b10) == 0, false)) {
    sample.timestamp() = 0;
  }

  if (__builtin_expect((parsed & 0b01) == 0, false)) {
    sample.value() = 0.0;
  }
}

template <class ProtobufReader, class Label>
PROMPP_ALWAYS_INLINE void read_label(ProtobufReader& pb_label, Label& label) {
  uint8_t parsed = 0;

  while (pb_label.next()) {
    switch (pb_label.tag()) {
      case 1:  // label name
        std::get<0>(label) = pb_label.get_view();
        parsed |= 0b01;
        break;
      case 2:  // label value
        std::get<1>(label) = pb_label.get_view();
        parsed |= 0b10;
        break;
      default:
        pb_label.skip();
    }
  }

  if (__builtin_expect(parsed != 0b11, false)) {
    throw BareBones::Exception(0xf355fc833ca6be64, "Protobuf message has incomplete key-value pair");
  }
}

enum TimeseriesTag {
  kLabels = 1,
  kSamples = 2,
  kHistograms = 4,
};

template <class ProtobufReader, class LabelSet>
PROMPP_ALWAYS_INLINE void read_only_label_set(ProtobufReader& pb_timeseries, LabelSet& label_set) {
  while (pb_timeseries.next(kLabels)) {
    auto pb_label = pb_timeseries.get_message();
    typename LabelSet::label_type label;
    read_label(pb_label, label);
    label_set.add(label);
  }

  if (__builtin_expect(!label_set.size(), false)) {
    throw BareBones::Exception(0xea6db0e3b0bc6feb, "Protobuf message has an empty label set, can't read labels");
  }
}

template <class ProtobufReader, class Timeseries>
PROMPP_ALWAYS_INLINE void read_timeseries(ProtobufReader&& pb_timeseries, Timeseries& timeseries) {
  while (pb_timeseries.next()) {
    switch (pb_timeseries.tag()) {
      case kLabels: {  // label
        auto pb_label = pb_timeseries.get_message();
        typename Timeseries::label_set_type::label_type label;
        read_label(pb_label, label);
        timeseries.label_set().add(label);
      } break;

      case kSamples: {  // sample
        auto& samples = timeseries.samples();
        samples.resize(samples.size() + 1);
        auto pb_sample = pb_timeseries.get_message();
        try {
          read_sample(pb_sample, samples.back());
        } catch (...) {
          samples.resize(samples.size() - 1);
          throw;
        }
      } break;

      default:
        pb_timeseries.skip();
    }
  }

  if (__builtin_expect(!timeseries.label_set().size() || !timeseries.samples().size(), false)) {
    throw BareBones::Exception(0x75a82db7eb2779f1, "Protobuf message has no samples for label set");
  }
}

enum HistogramSpanTag : uint8_t {
  kSpanOffset = 1,
  kSpanLength = 2,
};

enum HistogramTag : uint8_t {
  kCountInt = 1,
  kCountFloat = 2,
  kSum = 3,
  kSchema = 4,
  kZeroThreshold = 5,
  kZeroCountInt = 6,
  kZeroCountFloat = 7,
  kNegativeSpans = 8,
  kNegativeDeltas = 9,
  kNegativeCounts = 10,
  kPositiveSpans = 11,
  kPositiveDeltas = 12,
  kPositiveCounts = 13,
  kResetHint = 14,
  kTimestamp = 15,
  kCustomValues = 16,
};

template <class ProtobufReader, class Span>
PROMPP_ALWAYS_INLINE void read_histogram_span(ProtobufReader& pb_span, Span& span) {
  uint8_t parsed = 0;

  while (pb_span.next()) {
    switch (pb_span.tag()) {
      case kSpanOffset: {
        span.offset = pb_span.get_sint32();
        parsed |= 0b01;
        break;
      }

      case kSpanLength: {
        span.length = pb_span.get_uint32();
        parsed |= 0b10;
        break;
      }

      default: {
        pb_span.skip();
      }
    }
  }

  if (parsed != 0b11) [[unlikely]] {
    throw BareBones::Exception(0xa3f2c8e91b047d56, "Protobuf message has incomplete histogram span");
  }
}

template <class ProtobufReader, class Histogram>
PROMPP_ALWAYS_INLINE void read_histogram_sample(ProtobufReader& pb_histogram, Histogram& histogram) {
  bool count_parsed = false;

  while (pb_histogram.next()) {
    switch (pb_histogram.tag()) {
      case kCountInt: {
        histogram.count.value = pb_histogram.get_uint64();
        histogram.type = Primitives::HistogramType::kInt;
        count_parsed = true;
        break;
      }

      case kCountFloat: {
        histogram.count.float_value = pb_histogram.get_double();
        histogram.type = Primitives::HistogramType::kFloat;
        count_parsed = true;
        break;
      }

      case kSum: {
        histogram.sum = pb_histogram.get_double();
        break;
      }

      case kSchema: {
        histogram.schema = static_cast<int8_t>(pb_histogram.get_sint32());
        break;
      }

      case kZeroThreshold: {
        histogram.zero_threshold = pb_histogram.get_double();
        break;
      }

      case kZeroCountInt: {
        histogram.zero_count.value = pb_histogram.get_uint64();
        break;
      }

      case kZeroCountFloat: {
        histogram.zero_count.float_value = pb_histogram.get_double();
        break;
      }

      case kNegativeSpans: {
        auto pb_span = pb_histogram.get_message();
        read_histogram_span(pb_span, histogram.negative_spans.emplace_back());
        break;
      }

      case kNegativeDeltas: {
        histogram.negative_buckets.clear();
        for (const auto delta : pb_histogram.get_packed_sint64()) {
          histogram.negative_buckets.emplace_back().value = delta;
        }
        break;
      }

      case kNegativeCounts: {
        histogram.negative_buckets.clear();
        for (const auto count : pb_histogram.get_packed_double()) {
          histogram.negative_buckets.emplace_back().float_value = count;
        }
        break;
      }

      case kPositiveSpans: {
        auto pb_span = pb_histogram.get_message();
        read_histogram_span(pb_span, histogram.positive_spans.emplace_back());
        break;
      }

      case kPositiveDeltas: {
        histogram.positive_buckets.clear();
        for (const auto delta : pb_histogram.get_packed_sint64()) {
          histogram.positive_buckets.emplace_back().value = delta;
        }
        break;
      }

      case kPositiveCounts: {
        histogram.positive_buckets.clear();
        for (const auto count : pb_histogram.get_packed_double()) {
          histogram.positive_buckets.emplace_back().float_value = count;
        }
        break;
      }

      case kResetHint: {
        histogram.counter_reset_hint = static_cast<Primitives::CounterResetHint>(pb_histogram.get_enum());
        break;
      }

      case kTimestamp: {
        histogram.timestamp = pb_histogram.get_int64();
        break;
      }

      case kCustomValues: {
        histogram.custom_values.clear();
        for (const auto value : pb_histogram.get_packed_double()) {
          histogram.custom_values.emplace_back(value);
        }
        break;
      }

      default: {
        pb_histogram.skip();
      }
    }
  }

  if (!count_parsed) [[unlikely]] {
    throw BareBones::Exception(0xb4e3d9f02c158e67, "Protobuf message has incomplete histogram");
  }
}

template <class ProtobufReader, class Histogram>
PROMPP_ALWAYS_INLINE void read_histogram(ProtobufReader&& pb_timeseries, Histogram& histogram) {
  bool has_histogram = false;

  while (pb_timeseries.next()) {
    switch (pb_timeseries.tag()) {
      case kLabels: {
        auto pb_label = pb_timeseries.get_message();
        typename std::remove_cvref_t<decltype(histogram.label_set)>::label_type label;
        read_label(pb_label, label);
        histogram.label_set.add(label);
        break;
      }

      case kHistograms: {
        if (has_histogram) [[unlikely]] {
          throw BareBones::Exception(0xd6a5f1b24e37a089, "Protobuf message has multiple histograms for label set");
        }

        auto pb_histogram = pb_timeseries.get_message();
        read_histogram_sample(pb_histogram, histogram);
        has_histogram = true;
        break;
      }

      default: {
        pb_timeseries.skip();
      }
    }
  }

  if (histogram.label_set.empty() || !has_histogram) {
    throw BareBones::Exception(0xc5f4e0a13d269f78, "Protobuf message has no histograms for label set");
  }
}

template <class Timeseries, class ProtobufReader, class Callback>
  requires std::is_invocable<Callback, const Timeseries&>::value
void read_many_timeseries(ProtobufReader& pb, Callback func) {
  Timeseries timeseries;

  try {
    while (pb.next(1)) {
      auto pb_timeseries = pb.get_message();
      read_timeseries(pb_timeseries, timeseries);
      func(timeseries);
      timeseries.clear();
    }
  } catch (protozero::exception& e) {
    throw BareBones::Exception(0xf5386714f93eb11f, "Protobuf parsing timeseries exception: %s", e.what());
  }
}

// TODO: maybe delete it?
template <class ProtobufReader, class Timeseries>
PROMPP_ALWAYS_INLINE void read_timeseries_without_samples(ProtobufReader&& pb_timeseries, Timeseries& timeseries, const PbLabelSetMemoryLimits& limits) {
  size_t current_message_n = 0;
  while (pb_timeseries.next(kLabels)) {
    if (limits.max_label_names_per_timeseries && current_message_n >= limits.max_label_names_per_timeseries) {
      throw BareBones::Exception(0xf666cea4f74038c7, "Max Label Names count per Timeseries limit exceeded");
    }

    auto pb_label = pb_timeseries.get_message();
    typename Timeseries::label_set_type::label_type label;
    read_label(pb_label, label);
    if (size_t label_name_sz = std::size(std::get<0>(label)); limits.max_label_name_length && label_name_sz > limits.max_label_name_length) {
      throw BareBones::Exception(0x01102a3321345745, "Label name size (%zd) exceeds the maximum name size limit", label_name_sz);
    }
    if (size_t label_value_sz = std::size(std::get<1>(label)); limits.max_label_value_length && label_value_sz > limits.max_label_value_length) {
      throw BareBones::Exception(0x32b5ff9563758da8, "Label value size (%zd) exceeds the maximum value size limit", label_value_sz);
    }
    timeseries.label_set().add(label);

    current_message_n++;
  }

  if (__builtin_expect(!timeseries.label_set().size(), false)) {
    throw BareBones::Exception(0x68997b7d2e49de1e, "Protobuf message has an empty label set, can't read timeseries");
  }
}

enum class MetricsType : uint8_t {
  kUnknown = 0,
  kFloat,
  kHistogram,
};

template <class LabelSet>
PROMPP_ALWAYS_INLINE void read_and_validate_label(protozero::pbf_reader& reader, LabelSet& label_set, const PbLabelSetMemoryLimits& limits) {
  if (limits.max_label_names_per_timeseries_exceeded(label_set)) [[unlikely]] {
    throw BareBones::Exception(0xf666cea4f74038c7, "Max Label Names count per Timeseries limit exceeded");
  }

  auto pb_label = reader.get_message();
  typename LabelSet::label_type label;
  read_label(pb_label, label);
  if (const auto label_name = std::get<0>(label); limits.max_label_name_length_exceeded(label_name)) [[unlikely]] {
    throw BareBones::Exception(0x01102a3321345745, "Label name size (%zd) exceeds the maximum name size limit", label_name.size());
  }
  if (const auto label_value = std::get<1>(label); limits.max_label_value_length_exceeded(label_value)) [[unlikely]] {
    throw BareBones::Exception(0x32b5ff9563758da8, "Label value size (%zd) exceeds the maximum value size limit", label_value.size());
  }

  label_set.add(label);
}

template <class LabelSet>
PROMPP_ALWAYS_INLINE MetricsType preparse_timeseries(protozero::pbf_reader&& reader, LabelSet& label_set, const PbLabelSetMemoryLimits& limits) {
  auto metrics_type{MetricsType::kUnknown};
  while (reader.next()) {
    switch (reader.tag()) {
      case kLabels: {
        read_and_validate_label(reader, label_set, limits);
        break;
      }

      case kSamples: {
        metrics_type = MetricsType::kFloat;
        reader.skip();
        break;
      }

      case kHistograms: {
        metrics_type = MetricsType::kHistogram;
        reader.skip();
        break;
      }

      default: {
        reader.skip();
      }
    }
  }

  if (label_set.empty()) [[unlikely]] {
    throw BareBones::Exception(0x68997b7d2e49de1e, "Protobuf message has an empty label set, can't read timeseries");
  }

  return metrics_type;
}

template <class Timeseries, class Hashdex, class ProtobufReader>
  requires std::is_same<typename Hashdex::value_type, TimeseriesProtobufHashdexRecord>::value
void read_many_timeseries_in_hashdex(ProtobufReader& pb, Hashdex& hdx, const PbLabelSetMemoryLimits& limits) {
  Timeseries timeseries;
  size_t current_timeseries_n = 0;
  try {
    while (pb.next(1)) {
      if (limits.max_timeseries_count && current_timeseries_n >= limits.max_timeseries_count) {
        throw BareBones::Exception(0xdedb5b24d946cc4d, "Max Timeseries count limit exceeded");
        break;
      }
      auto pb_view = pb.get_view();
      read_timeseries_without_samples(protozero::pbf_reader{pb_view}, timeseries, limits);
      hdx.emplace_back(hash_value(timeseries.label_set()), pb_view);
      timeseries.clear();
      current_timeseries_n++;
    }
  } catch (protozero::exception& e) {
    throw BareBones::Exception(0xbe40bda82f01b869, "Protobuf parsing timeseries exception: %s", e.what());
  }
}

}  // namespace PromPP::Prometheus::RemoteWrite