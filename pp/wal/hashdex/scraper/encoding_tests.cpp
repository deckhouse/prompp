#include <gtest/gtest.h>

#include <cmath>
#include <cstdint>
#include <cstring>
#include <type_traits>

#include "encoding.h"
#include "primitives/sample.h"
#include "prometheus/value.h"

namespace {

using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::WAL::hashdex::scraper::encoding::LabelCodec;
using PromPP::WAL::hashdex::scraper::encoding::LayoutMarker;
using PromPP::WAL::hashdex::scraper::encoding::SampleCodec;
using PromPP::WAL::hashdex::scraper::encoding::SampleValueType;

struct ValueTypeCase {
  double input;
  SampleValueType expected;
};

class ValueTypeFixture : public testing::TestWithParam<ValueTypeCase> {};

TEST_P(ValueTypeFixture, ClassifyValueCorrectly) {
  // Arrange

  // Act
  const auto actual = SampleCodec::value_type(GetParam().input);

  // Assert
  EXPECT_EQ(actual, GetParam().expected);
}

INSTANTIATE_TEST_SUITE_P(ValueTypeTests,
                         ValueTypeFixture,
                         testing::Values(ValueTypeCase{.input = PromPP::Prometheus::kNormalNan, .expected = SampleValueType::kNaN},
                                         ValueTypeCase{.input = PromPP::Prometheus::kStaleNan, .expected = SampleValueType::kNaN},
                                         ValueTypeCase{.input = std::numeric_limits<double>::quiet_NaN(), .expected = SampleValueType::kNaN},

                                         ValueTypeCase{.input = 0.0, .expected = SampleValueType::kZero},
                                         ValueTypeCase{.input = -0.0, .expected = SampleValueType::kZero},

                                         ValueTypeCase{.input = 1.0, .expected = SampleValueType::kUint8},
                                         ValueTypeCase{.input = 255.0, .expected = SampleValueType::kUint8},

                                         ValueTypeCase{.input = 256.0, .expected = SampleValueType::kUint16},
                                         ValueTypeCase{.input = 65535.0, .expected = SampleValueType::kUint16},

                                         ValueTypeCase{.input = 65536.0, .expected = SampleValueType::kUint32},
                                         ValueTypeCase{.input = 4294967295.0, .expected = SampleValueType::kUint32},

                                         ValueTypeCase{.input = 3.5, .expected = SampleValueType::kFloat},
                                         ValueTypeCase{.input = -42.0, .expected = SampleValueType::kFloat},

                                         ValueTypeCase{.input = 1e-10, .expected = SampleValueType::kDouble},
                                         ValueTypeCase{.input = 3.141592653589793, .expected = SampleValueType::kDouble},
                                         ValueTypeCase{.input = 0.1, .expected = SampleValueType::kDouble}));

struct SampleCodecCase {
  Sample sample;
  bool has_ts;
};

class SampleCodecFixture : public testing::TestWithParam<SampleCodecCase> {
 protected:
  static constexpr size_t kBufSize = 64;

  SampleCodec::DecodeResult encode_and_decode(const LayoutMarker layout, Sample sample, int64_t default_ts = -1) {
    buf_.fill(0);
    char* start = buf_.data();
    char* end = SampleCodec::encode(start, layout, sample);

    const auto res = SampleCodec::decode(start, layout, default_ts);

    EXPECT_EQ(res.next, end);
    return res;
  }

  std::array<char, kBufSize> buf_{};
  const Timestamp default_ts_ = -1;
};

TEST_P(SampleCodecFixture, CorrectSample) {
  // Arrange
  const auto layout = LayoutMarker::make(GetParam().has_ts, 0, SampleCodec::value_type(GetParam().sample.value()));

  Sample expected = GetParam().sample;
  if (!GetParam().has_ts) {
    expected.timestamp() = default_ts_;
  }

  // Act
  const auto res = encode_and_decode(layout, GetParam().sample, default_ts_);

  // Assert
  EXPECT_EQ(res.sample, expected);
}

INSTANTIATE_TEST_SUITE_P(SampleCodecTests,
                         SampleCodecFixture,
                         testing::Values(SampleCodecCase{.sample = Sample(42, 123.0), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(12345, std::numbers::pi), .has_ts = false},
                                         SampleCodecCase{.sample = Sample(111, 1.2345), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(7, 255.0), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(9, 65535.0), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(222, 0.0), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(333, PromPP::Prometheus::kNormalNan), .has_ts = true},
                                         SampleCodecCase{.sample = Sample(1000, 42.0), .has_ts = false},
                                         SampleCodecCase{.sample = Sample(2048, 4242.0), .has_ts = true}));

class LabelCodecFixture : public testing::Test {
 protected:
  static constexpr size_t kBufSize = 64;
  std::array<char, kBufSize> buf_{};

  LabelCodec::DecodeResult encode_and_decode(uint32_t name_off, uint32_t name_len, uint32_t value_off, uint32_t value_len) {
    buf_.fill(0);
    char* start = buf_.data();
    char* end = LabelCodec::encode(start, name_off, name_len, value_off, value_len);

    const auto res = LabelCodec::decode(start);

    EXPECT_EQ(res.next, end);
    return res;
  }
};

TEST_F(LabelCodecFixture, FastPath_Layout01010101) {
  // Arrange
  buf_.fill(0);
  char* start = buf_.data();
  start[0] = static_cast<char>(0b01010101);
  start[1] = 10;
  start[2] = 11;
  start[3] = 12;
  start[4] = 13;

  // Act
  const auto res = LabelCodec::decode(buf_.data());

  // Assert
  EXPECT_EQ(res.label_name_offset, 10u);
  EXPECT_EQ(res.label_name_length, 11u);
  EXPECT_EQ(res.label_value_offset, 12u);
  EXPECT_EQ(res.label_value_length, 13u);
  EXPECT_EQ(res.next, buf_.data() + 5);
}

TEST_F(LabelCodecFixture, SimplifiedPath_NameFieldsZero) {
  // Arrange
  buf_.fill(0);
  uint8_t layout = 0b10010000;
  buf_[0] = static_cast<char>(layout);
  buf_[1] = 42;
  uint16_t len = 1234;
  std::memcpy(buf_.data() + 2, &len, 2);

  // Act
  const auto res = LabelCodec::decode(buf_.data());

  // Assert
  EXPECT_EQ(res.label_name_offset, 0u);
  EXPECT_EQ(res.label_name_length, 0u);
  EXPECT_EQ(res.label_value_offset, 42u);
  EXPECT_EQ(res.label_value_length, 1234u);
  EXPECT_EQ(res.next, buf_.data() + 1 + 1 + 2);
}

struct LabelCase {
  uint32_t name_off;
  uint32_t name_len;
  uint32_t value_off;
  uint32_t value_len;
};

class LabelCodecParamFixture : public LabelCodecFixture, public testing::WithParamInterface<LabelCase> {};

TEST_P(LabelCodecParamFixture, EncodeDecode) {
  // Arrange

  // Act
  const auto res = encode_and_decode(GetParam().name_off, GetParam().name_len, GetParam().value_off, GetParam().value_len);

  // Assert
  EXPECT_EQ(res.label_name_offset, GetParam().name_off);
  EXPECT_EQ(res.label_name_length, GetParam().name_len);
  EXPECT_EQ(res.label_value_offset, GetParam().value_off);
  EXPECT_EQ(res.label_value_length, GetParam().value_len);
}

INSTANTIATE_TEST_SUITE_P(LabelCodecTests,
                         LabelCodecParamFixture,
                         testing::Values(LabelCase{.name_off = 0, .name_len = 0, .value_off = 0, .value_len = 0},
                                         LabelCase{.name_off = 1, .name_len = 2, .value_off = 3, .value_len = 4},
                                         LabelCase{.name_off = 300, .name_len = 400, .value_off = 500, .value_len = 600},
                                         LabelCase{.name_off = 100000, .name_len = 200000, .value_off = 300000, .value_len = 400000},
                                         LabelCase{.name_off = 0, .name_len = 0, .value_off = 300000, .value_len = 400000},
                                         LabelCase{.name_off = 0, .name_len = 0, .value_off = 1234, .value_len = 123456},
                                         LabelCase{.name_off = 0, .name_len = 12, .value_off = 1234, .value_len = 123456}));

}  // namespace