#include <gtest/gtest.h>

#include <string>

#include "metric.h"
#include "protobuf.h"
#include "wal/hashdex/test_fixture.h"

#include "primitives/label_set.h"
#include "primitives/sample.h"
#include "third_party/protozero/pbf_writer.hpp"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Prometheus::MetadataType;
using PromPP::WAL::hashdex::FloatMetric;
using PromPP::WAL::hashdex::Metadata;
using std::operator""sv;

struct ProtobufCase {
  std::string_view protobuf;
  std::vector<Metadata> metadata{};
  std::vector<FloatMetric> floats{};
};

class ProtobufFixture : public ::testing::TestWithParam<ProtobufCase> {
 protected:
  PromPP::WAL::hashdex::Protobuf hashdex_;

  void SetUp() override { calculate_labelset_hash(const_cast<ProtobufCase&>(GetParam()).floats); }
};

TEST_P(ProtobufFixture, Test) {
  // Arrange

  // Act
  hashdex_.presharding(GetParam().protobuf, false);
  const auto floats = get_floats(hashdex_);
  const auto metadata = get_metadata(hashdex_);

  // Assert
  EXPECT_EQ(GetParam().floats, floats);
  EXPECT_EQ(GetParam().metadata, metadata);
}

INSTANTIATE_TEST_SUITE_P(
    Tests,
    ProtobufFixture,
    testing::Values(ProtobufCase{
        .protobuf =
            "\x0a\x58\x0a\x10\x0a\x08\x5f\x5f\x6e\x61\x6d\x65\x5f\x5f\x12\x04\x74\x65\x73\x74\x0a\x14\x0a\x07\x63\x6c\x75\x73\x74\x65\x72\x12\x09\x63\x6c\x75"
            "\x73\x74\x65\x72\x2d\x30\x0a\x18\x0a\x0b\x5f\x5f\x72\x65\x70\x6c\x69\x63\x61\x5f\x5f\x12\x09\x72\x65\x70\x6c\x69\x63\x61\x2d\x31\x12\x14\x09\x00"
            "\x00\x00\x00\x00\x5c\xb1\x40\x10\xe0\xc6\x9c\x8d\xec\xcf\xff\xff\xff\x01\x1a\x15\x12\x05\x74\x65\x73\x74\x31\x22\x0c\x74\x65\x73\x74\x20\x63\x6f"
            "\x75\x6e\x74\x65\x72\x1a\x12\x12\x05\x74\x65\x73\x74\x32\x2a\x09\x74\x65\x73\x74\x20\x75\x6e\x69\x74\x1a\x09\x08\x01\x12\x05\x74\x65\x73\x74\x33"sv,
        .metadata = {Metadata{.metric_name = "test1", .text = "test counter", .type = MetadataType::kHelp},
                     Metadata{.metric_name = "test2", .text = "test unit", .type = MetadataType::kUnit},
                     Metadata{.metric_name = "test3", .text = "COUNTER", .type = MetadataType::kType}},
        .floats = {FloatMetric{.timeseries = {LabelViewSet{
                                                  {"__name__", "test"},
                                                  {"__replica__", "replica-1"},
                                                  {"cluster", "cluster-0"},
                                              },
                                              BareBones::Vector<Sample>{Sample{-1654608420000, 4444}}}}}}));

TEST(ProtobufNoSamples, ThrowsWhenTimeseriesHasNoSamples) {
  // Arrange
  constexpr auto protobuf =
      "\x0a\x27\x0a\x1b\x0a\x08\x5f\x5f\x6e\x61\x6d\x65\x5f\x5f\x12\x0f\x74\x65\x73\x74\x5f\x6e\x6f\x5f\x73\x61\x6d\x70\x6c\x65\x73\x0a\x08\x0a\x03\x6a\x6f"
      "\x62\x12\x01\x78"sv;
  PromPP::WAL::hashdex::Protobuf hashdex;

  // Act & Assert
  EXPECT_THROW(hashdex.presharding(protobuf, false), BareBones::Exception);
}

TEST(ProtobufNoSamples, DoesNotThrowWhenSkippingTimeseriesWithNoSamples) {
  // Arrange
  constexpr auto protobuf =
      "\x0a\x27\x0a\x1b\x0a\x08\x5f\x5f\x6e\x61\x6d\x65\x5f\x5f\x12\x0f\x74\x65\x73\x74\x5f\x6e\x6f\x5f\x73\x61\x6d\x70\x6c\x65\x73\x0a\x08\x0a\x03\x6a\x6f"
      "\x62\x12\x01\x78"sv;
  PromPP::WAL::hashdex::Protobuf hashdex;

  // Act & Assert
  EXPECT_NO_THROW(hashdex.presharding(protobuf, true));
  EXPECT_TRUE(get_floats(hashdex).empty());
}

TEST(ProtobufDuplicateLabelNames, RejectsSeriesWithoutSamplesEvenWhenSkippingIsEnabled) {
  // Arrange
  std::string protobuf;
  {
    protozero::pbf_writer writer(protobuf);
    protozero::pbf_writer series(writer, 1);
    PromPP::Prometheus::RemoteWrite::write_label(series, "__name__", "metric");
    PromPP::Prometheus::RemoteWrite::write_label(series, "job", "");
    PromPP::Prometheus::RemoteWrite::write_label(series, "job", "");
  }
  PromPP::WAL::hashdex::Protobuf hashdex;
  BareBones::Exception::Code error_code{};

  // Act
  try {
    hashdex.presharding(protobuf, true);
  } catch (const BareBones::Exception& error) {
    error_code = error.code();
  }

  // Assert
  EXPECT_EQ(0x456a9d8a8915042eULL, error_code);
  EXPECT_TRUE(hashdex.floats().empty());
}

TEST(ProtobufLabelNormalization, RemovesUniqueEmptyValueBeforeHashing) {
  // Arrange
  std::string protobuf;
  {
    protozero::pbf_writer writer(protobuf);
    protozero::pbf_writer series(writer, 1);
    PromPP::Prometheus::RemoteWrite::write_label(series, "job", "api");
    PromPP::Prometheus::RemoteWrite::write_label(series, "instance", "");
    PromPP::Prometheus::RemoteWrite::write_sample(series, Sample{1, 1.0});
  }
  PromPP::WAL::hashdex::Protobuf hashdex;
  const LabelViewSet expected{{"job", "api"}};

  // Act
  hashdex.presharding(protobuf, false);
  const auto floats = get_floats(hashdex);

  // Assert
  ASSERT_EQ(1U, floats.size());
  EXPECT_EQ(expected, floats.front().timeseries.label_set());
  EXPECT_EQ(hash_value(expected), floats.front().hash);
}

std::string make_wide_protobuf_series() {
  std::string protobuf;
  {
    protozero::pbf_writer writer(protobuf);
    protozero::pbf_writer series(writer, 1);
    PromPP::Prometheus::RemoteWrite::write_label(series, "__name__", "metric");
    PromPP::Prometheus::RemoteWrite::write_label(series, "label_1000", "east");
    for (unsigned index = 1001; index <= 1300; ++index) {
      PromPP::Prometheus::RemoteWrite::write_label(series, "label_" + std::to_string(index), "");
    }
    PromPP::Prometheus::RemoteWrite::write_label(series, "job", "api");
    PromPP::Prometheus::RemoteWrite::write_sample(series, Sample{1, 1.0});
  }
  return protobuf;
}

TEST(ProtobufLabelNormalization, PreservesHashWithWideLabelSet) {
  // Arrange
  const std::string protobuf = make_wide_protobuf_series();
  PromPP::WAL::hashdex::Protobuf hashdex;
  const LabelViewSet expected{{"__name__", "metric"}, {"job", "api"}, {"label_1000", "east"}};

  // Act
  hashdex.presharding(protobuf, false);
  const auto floats = get_floats(hashdex);

  // Assert
  ASSERT_EQ(1U, floats.size());
  EXPECT_EQ(expected, floats.front().timeseries.label_set());
  EXPECT_EQ(hash_value(expected), floats.front().hash);
}

}  // namespace
