#include <gtest/gtest.h>

#include <cmath>
#include <cstdint>
#include <cstring>
#include <type_traits>

#include "encoding.h"
#include "primitives/sample.h"
#include "prometheus/value.h"

namespace {

using namespace PromPP::WAL::hashdex::scraper::encoding;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;

struct ValueTypeCase {
  double input;
  SampleValueType expected;
};

class ValueTypeFixture : public testing::TestWithParam<ValueTypeCase> {};

TEST_P(ValueTypeFixture, ClassifyValueCorrectly) {
  // Arrange

  // Act
  const auto actual = value_type(GetParam().input);

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

                                         ValueTypeCase{.input = 1e300, .expected = SampleValueType::kDouble},
                                         ValueTypeCase{.input = 3.141592653589793, .expected = SampleValueType::kDouble},
                                         ValueTypeCase{.input = 0.1, .expected = SampleValueType::kDouble}));

class SampleCodecFixture : public testing::Test {
 protected:
  static constexpr size_t kBufSize = 64;
  std::pair<char*, std::array<char, kBufSize>&> encode_into_buffer(const LayoutMarker layout, Sample sample) {
    buf_.fill(0);
    char* start = buf_.data();
    char* end = SampleCodec::encode(start, layout, sample);
    return {end, buf_};
  }

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

TEST_F(SampleCodecFixture, EncodeDecode_Uint32_WithTimestamp) {
  // Arrange
  Sample s{};
  s.value() = 123.0;
  s.timestamp() = int64_t{42};

  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kUint32);

  // Act
  auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), 123.0);
  EXPECT_EQ(res.sample.timestamp(), 42);
}

TEST_F(SampleCodecFixture, EncodeDecode_Double_WithoutTimestamp) {
  // Arrange
  Sample s{};
  s.value() = 3.141592653589793;
  s.timestamp() = int64_t{12345};

  const auto layout = LayoutMarker::make(false, 0, SampleValueType::kDouble);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), 3.141592653589793);
  EXPECT_EQ(res.sample.timestamp(), default_ts_);
}

TEST_F(SampleCodecFixture, EncodeDecode_Float_WithTimestamp) {
  // Arrange
  Sample s{};
  s.value() = 1.2345;
  s.timestamp() = int64_t{111};

  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kFloat);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), static_cast<double>(static_cast<float>(1.2345)));
  EXPECT_EQ(res.sample.timestamp(), 111);
}

TEST_F(SampleCodecFixture, EncodeDecode_Uint8) {
  // Arrange
  Sample s{};
  s.value() = 255.0;
  s.timestamp() = 7;
  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kUint8);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), 255.0);
  EXPECT_EQ(res.sample.timestamp(), 7);
}

TEST_F(SampleCodecFixture, EncodeDecode_Uint16) {
  // Arrange
  Sample s{};
  s.value() = 65535.0;
  s.timestamp() = 9;
  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kUint16);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), 65535.0);
  EXPECT_EQ(res.sample.timestamp(), 9);
}

TEST_F(SampleCodecFixture, EncodeDecode_ZeroType_IgnoresOriginalValue) {
  // Arrange
  Sample s{};
  s.value() = 12345.0;
  s.timestamp() = 222;
  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kZero);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_DOUBLE_EQ(res.sample.value(), 0.0);
  EXPECT_EQ(res.sample.timestamp(), 222);
}

TEST_F(SampleCodecFixture, Decode_NaNType_ProducesNaN) {
  // Arrange
  Sample s{};
  s.value() = 777.0;
  s.timestamp() = 333;
  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kNaN);

  // Act
  const auto res = encode_and_decode(layout, s, default_ts_);

  // Assert
  EXPECT_TRUE(std::isnan(res.sample.value()));
  EXPECT_EQ(res.sample.timestamp(), 333);
}

TEST_F(SampleCodecFixture, Decode_ReturnedPointerAdvancesCorrectly_NoTimestamp) {
  // Arrange
  Sample s{};
  s.value() = 42.0;
  s.timestamp() = 1000;

  const auto layout = LayoutMarker::make(false, 0, SampleValueType::kUint32);

  buf_.fill(0);
  char* start = buf_.data();
  char* end = SampleCodec::encode(start, layout, s);

  // Act
  const auto res = SampleCodec::decode(start, layout, default_ts_);

  // Assert
  EXPECT_EQ(res.next, end);
  EXPECT_DOUBLE_EQ(res.sample.value(), 42.0);
  EXPECT_EQ(res.sample.timestamp(), default_ts_);
}

TEST_F(SampleCodecFixture, Decode_ReturnedPointerAdvancesCorrectly_WithTimestamp) {
  // Arrange
  Sample s{};
  s.value() = 4242.0;
  s.timestamp() = 2048;

  const auto layout = LayoutMarker::make(true, 0, SampleValueType::kDouble);

  buf_.fill(0);
  char* start = buf_.data();
  char* end = SampleCodec::encode(start, layout, s);

  // Act
  const auto res = SampleCodec::decode(start, layout, default_ts_);

  // Assert
  EXPECT_EQ(res.next, end);
  EXPECT_DOUBLE_EQ(res.sample.value(), 4242.0);
  EXPECT_EQ(res.sample.timestamp(), 2048);
}

}  // namespace