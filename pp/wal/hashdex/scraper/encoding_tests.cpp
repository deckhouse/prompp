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
  const auto actual = PromPP::WAL::hashdex::scraper::encoding::value_type(GetParam().input);

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

TEST_F(LabelCodecFixture, EncodeDecode_AllZeros) {
  // Arrange

  // Act
  const auto res = encode_and_decode(0, 0, 0, 0);

  // Assert
  EXPECT_EQ(res.label_name_offset, 0u);
  EXPECT_EQ(res.label_name_length, 0u);
  EXPECT_EQ(res.label_value_offset, 0u);
  EXPECT_EQ(res.label_value_length, 0u);
}

TEST_F(LabelCodecFixture, EncodeDecode_OneByteValues) {
  // Arrange

  // Act
  const auto res = encode_and_decode(1, 2, 3, 4);

  EXPECT_EQ(res.label_name_offset, 1u);
  EXPECT_EQ(res.label_name_length, 2u);
  EXPECT_EQ(res.label_value_offset, 3u);
  EXPECT_EQ(res.label_value_length, 4u);
}

TEST_F(LabelCodecFixture, EncodeDecode_TwoByteValues) {
  // Arrange

  // Act
  const auto res = encode_and_decode(300, 400, 500, 600);

  EXPECT_EQ(res.label_name_offset, 300u);
  EXPECT_EQ(res.label_name_length, 400u);
  EXPECT_EQ(res.label_value_offset, 500u);
  EXPECT_EQ(res.label_value_length, 600u);
}

TEST_F(LabelCodecFixture, EncodeDecode_FourByteValues) {
  // Arrange

  // Act
  const auto res = encode_and_decode(100000, 200000, 300000, 400000);

  // Assert
  EXPECT_EQ(res.label_name_offset, 100000u);
  EXPECT_EQ(res.label_name_length, 200000u);
  EXPECT_EQ(res.label_value_offset, 300000u);
  EXPECT_EQ(res.label_value_length, 400000u);
}

TEST_F(LabelCodecFixture, EncodeDecode_0044) {
  // Arrange

  // Act
  const auto res = encode_and_decode(0, 0, 300000, 400000);

  // Assert
  EXPECT_EQ(res.label_name_offset, 0);
  EXPECT_EQ(res.label_name_length, 0);
  EXPECT_EQ(res.label_value_offset, 300000u);
  EXPECT_EQ(res.label_value_length, 400000u);
}

TEST_F(LabelCodecFixture, EncodeDecode_0024) {
  // Arrange

  // Act
  const auto res = encode_and_decode(0, 0, 1234, 123456);

  // Assert
  EXPECT_EQ(res.label_name_offset, 0);
  EXPECT_EQ(res.label_name_length, 0);
  EXPECT_EQ(res.label_value_offset, 1234);
  EXPECT_EQ(res.label_value_length, 123456);
}

TEST_F(LabelCodecFixture, EncodeDecode_0124) {
  // Arrange

  // Act
  const auto res = encode_and_decode(0, 12, 1234, 123456);

  // Assert
  EXPECT_EQ(res.label_name_offset, 0);
  EXPECT_EQ(res.label_name_length, 12);
  EXPECT_EQ(res.label_value_offset, 1234);
  EXPECT_EQ(res.label_value_length, 123456);
}

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

}  // namespace