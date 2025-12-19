#include <gtest/gtest.h>

#include <ranges>
#include <spanstream>
#include <sstream>
#include <string>
#include <tuple>
#include <vector>

#include "wal/wal.h"

namespace {

using PromPP::Primitives::LabelSet;
using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap;
using PromPP::WAL::BasicDecoder;
using PromPP::WAL::BasicEncoder;

struct SeriesSample {
  Sample sample;
  LabelSetID ls_id;

  bool operator==(const SeriesSample&) const noexcept = default;
};

struct WalBufferAddCase {
  std::vector<SeriesSample> samples;
  std::vector<SeriesSample> expected_samples;
  uint32_t samples_count;
  uint32_t series_count;
  Timestamp earliest_sample;
  Timestamp latest_sample;
};

class WalBufferAddFixture : public testing::TestWithParam<WalBufferAddCase> {
 protected:
  BasicEncoder<PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<BareBones::Vector>>::Buffer buffer_;

  void add() {
    for (auto& series_sample : GetParam().samples) {
      buffer_.add(series_sample.ls_id, series_sample.sample);
    }
  }

  [[nodiscard]] std::vector<SeriesSample> get() const noexcept {
    std::vector<SeriesSample> samples;
    buffer_.for_each([&samples](LabelSetID ls_id, Timestamp timestamp, Sample::value_type value) {
      samples.emplace_back(SeriesSample{.sample = {timestamp, value}, .ls_id = ls_id});
    });
    return samples;
  }
};

TEST_P(WalBufferAddFixture, TestAdd) {
  // Arrange

  // Act
  add();

  // Assert
  EXPECT_EQ(GetParam().samples_count, buffer_.samples_count());
  EXPECT_EQ(GetParam().series_count, buffer_.series_count());
  EXPECT_EQ(GetParam().earliest_sample, buffer_.earliest_sample());
  EXPECT_EQ(GetParam().latest_sample, buffer_.latest_sample());
  EXPECT_EQ(GetParam().expected_samples, get());
}

TEST_F(WalBufferAddFixture, TestFillFirstSampleAddedAtTsNs) {
  // Arrange
  const auto start = buffer_.first_sample_added_at_ts_ns();

  // Act
  buffer_.add(0, Sample{101, 1.0});
  const auto filled = buffer_.first_sample_added_at_ts_ns();
  buffer_.add(0, Sample{102, 1.0});

  // Assert
  EXPECT_EQ(0, start);
  EXPECT_EQ(filled, buffer_.first_sample_added_at_ts_ns());
  EXPECT_NE(0, filled);
}

TEST_F(WalBufferAddFixture, Clear) {
  // Arrange
  buffer_.add(0, Sample{101, 1.0});

  // Act
  buffer_.clear();

  // Assert
  EXPECT_EQ(0U, buffer_.samples_count());
  EXPECT_EQ(0U, buffer_.series_count());
  EXPECT_EQ(std::numeric_limits<Timestamp>::max(), buffer_.earliest_sample());
  EXPECT_EQ(0, buffer_.latest_sample());
  EXPECT_EQ(0, buffer_.first_sample_added_at_ts_ns());
  EXPECT_EQ(std::vector<SeriesSample>{}, get());
}

INSTANTIATE_TEST_SUITE_P(OneSample,
                         WalBufferAddFixture,
                         testing::Values(WalBufferAddCase{
                             .samples = {{.sample = {101, 1.0}, .ls_id = 0}},
                             .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}},
                             .samples_count = 1,
                             .series_count = 1,
                             .earliest_sample = 101,
                             .latest_sample = 101,
                         }));

INSTANTIATE_TEST_SUITE_P(
    ManySamplesForOneSerie,
    WalBufferAddFixture,
    testing::Values(
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {103, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {103, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 103,
        }));

INSTANTIATE_TEST_SUITE_P(
    SortSamplesByTimestamp,
    WalBufferAddFixture,
    testing::Values(
        WalBufferAddCase{
            .samples = {{.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {100, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {100, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 100,
            .latest_sample = 102,
        }));

INSTANTIATE_TEST_SUITE_P(
    TwoSeries,
    WalBufferAddFixture,
    testing::Values(
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1}, {.sample = {101, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 1}},
            .samples_count = 2,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 101,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1000}, {.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 1.0}, .ls_id = 1000}},
            .samples_count = 3,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 1000},
                        {.sample = {102, 1.0}, .ls_id = 1000},
                        {.sample = {101, 1.0}, .ls_id = 0},
                        {.sample = {102, 1.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0},
                                 {.sample = {102, 1.0}, .ls_id = 0},
                                 {.sample = {101, 1.0}, .ls_id = 1000},
                                 {.sample = {102, 1.0}, .ls_id = 1000}},
            .samples_count = 4,
            .series_count = 2,
            .earliest_sample = 101,
            .latest_sample = 102,
        }));

INSTANTIATE_TEST_SUITE_P(
    DontSkipNonUniqueSamples,
    WalBufferAddFixture,
    testing::Values(
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .samples_count = 2,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 101,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {101, 2.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        },
        WalBufferAddCase{
            .samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {102, 2.0}, .ls_id = 0}},
            .expected_samples = {{.sample = {101, 1.0}, .ls_id = 0}, {.sample = {102, 1.0}, .ls_id = 0}, {.sample = {102, 2.0}, .ls_id = 0}},
            .samples_count = 3,
            .series_count = 1,
            .earliest_sample = 101,
            .latest_sample = 102,
        }));

class WalEncoderDecoderFixture : public testing::Test {
 protected:
  using WALEncoder = BasicEncoder<>;
  using WALDecoder = BasicDecoder<EncodingBimap<BareBones::Vector>>;

  struct DecodedSample {
    uint32_t ls_id;
    Timestamp timestamp;
    double value;

    bool operator==(const DecodedSample& other) const noexcept {
      return ls_id == other.ls_id && timestamp == other.timestamp && std::bit_cast<uint64_t>(value) == std::bit_cast<uint64_t>(other.value);
    }
  };

  static PromPP::Primitives::Timeseries create_timeseries(const LabelSet& label_set, const std::vector<Sample>& samples) {
    PromPP::Primitives::Timeseries timeseries;
    timeseries.label_set().add(label_set);
    for (const auto& sample : samples) {
      timeseries.samples().emplace_back(sample);
    }
    return timeseries;
  }

  static std::vector<DecodedSample> collect_samples_from_buffer(const WALEncoder::Buffer& buffer) {
    std::vector<DecodedSample> samples;
    buffer.for_each([&](uint32_t ls_id, Timestamp timestamp, double value) { samples.emplace_back(DecodedSample{ls_id, timestamp, value}); });
    return samples;
  }

  static bool contains_sample(const std::vector<DecodedSample>& samples, uint32_t ls_id, Timestamp timestamp, double value) {
    const DecodedSample expected{.ls_id = ls_id, .timestamp = timestamp, .value = value};
    return std::ranges::find(samples, expected) != samples.end();
  }

  static bool contains_stale_nan_sample(const std::vector<DecodedSample>& samples, uint32_t ls_id, Timestamp timestamp) {
    const auto stale_nan = std::bit_cast<uint64_t>(BareBones::Encoding::Gorilla::STALE_NAN);
    return std::ranges::find_if(samples, [=](const DecodedSample& s) {
             return s.ls_id == ls_id && s.timestamp == timestamp && std::bit_cast<uint64_t>(s.value) == stale_nan;
           }) != samples.end();
  }
};

TEST_F(WalEncoderDecoderFixture, AddTimeseriesToEncoder) {
  // Arrange
  const LabelSet label_set{{"metric", "cpu_usage"}, {"instance", "server1"}};
  const std::vector<Sample> samples{{1000, 1.5}, {2000, 2.0}, {3000, 1.8}};
  const auto timeseries = create_timeseries(label_set, samples);
  WALEncoder encoder;

  // Act
  encoder.add(timeseries);

  // Assert
  EXPECT_EQ(encoder.buffer().samples_count(), 3U);
  EXPECT_EQ(encoder.buffer().series_count(), 1U);
  EXPECT_EQ(encoder.buffer().earliest_sample(), 1000);
  EXPECT_EQ(encoder.buffer().latest_sample(), 3000);
  EXPECT_EQ(encoder.samples(), 0U);
}

TEST_F(WalEncoderDecoderFixture, EncoderStoresLabelSet) {
  // Arrange
  const LabelSet label_set{{"metric", "cpu_usage"}, {"instance", "server1"}};
  const std::vector<Sample> samples{{1000, 1.5}, {2000, 2.0}};
  const auto timeseries = create_timeseries(label_set, samples);
  WALEncoder encoder;

  // Act
  encoder.add(timeseries);

  // Assert
  ASSERT_EQ(encoder.label_sets().size(), 1U);
  EXPECT_TRUE(std::ranges::equal(label_set, encoder.label_sets()[0]));
}

TEST_F(WalEncoderDecoderFixture, EncodeDecodeLabelSet) {
  // Arrange
  const LabelSet label_set{{"metric", "cpu_usage"}, {"instance", "server1"}};
  const std::vector<Sample> samples{{1000, 1.5}, {2000, 2.0}, {3000, 1.8}};
  const auto timeseries = create_timeseries(label_set, samples);
  WALEncoder encoder;
  encoder.add(timeseries);

  EncodingBimap<BareBones::Vector> encoding_bimap;
  WALDecoder decoder{encoding_bimap, PromPP::WAL::BasicEncoderVersion::kV3};
  std::stringstream stream;

  // Act
  stream << encoder;
  stream >> decoder;

  // Assert
  EXPECT_EQ(encoder.samples(), 3U);
  ASSERT_EQ(decoder.label_sets().size(), 1U);
  EXPECT_TRUE(std::ranges::equal(label_set, decoder.label_sets()[0]));
}

TEST_F(WalEncoderDecoderFixture, EncodeDecodeSamples) {
  // Arrange
  const LabelSet label_set{{"metric", "cpu_usage"}};
  const std::vector<Sample> samples{{1000, 1.5}, {2000, 2.0}, {3000, 1.8}};
  const auto timeseries = create_timeseries(label_set, samples);
  WALEncoder encoder;
  encoder.add(timeseries);

  EncodingBimap<BareBones::Vector> encoding_bimap;
  WALDecoder decoder{encoding_bimap, PromPP::WAL::BasicEncoderVersion::kV3};
  std::stringstream stream;

  // Act
  stream << encoder;
  stream >> decoder;

  std::vector<Sample> decoded_samples;
  decoder.process_segment([&](uint32_t, uint64_t ts, double v) { decoded_samples.emplace_back(ts, v); });

  // Assert
  ASSERT_EQ(decoded_samples, samples);
}

TEST_F(WalEncoderDecoderFixture, EncodeDecodePreservesEarliestLatestSamples) {
  // Arrange
  const LabelSet label_set{{"metric", "cpu_usage"}};
  const std::vector<Sample> samples{{1000, 1.5}, {2000, 2.0}, {3000, 1.8}};
  const auto timeseries = create_timeseries(label_set, samples);

  WALEncoder encoder;
  encoder.add(timeseries);

  const auto earliest_before = encoder.buffer().earliest_sample();
  const auto latest_before = encoder.buffer().latest_sample();

  EncodingBimap<BareBones::Vector> encoding_bimap;
  WALDecoder decoder{encoding_bimap, PromPP::WAL::BasicEncoderVersion::kV3};
  std::stringstream stream;

  // Act
  stream << encoder;
  stream >> decoder;
  // Process segment to update earliest/latest
  decoder.process_segment([](uint32_t, uint64_t, double) {});

  // Assert
  EXPECT_EQ(decoder.earliest_sample(), earliest_before);
  EXPECT_EQ(decoder.latest_sample(), latest_before);
  EXPECT_EQ(decoder.samples(), 3U);
}

TEST_F(WalEncoderDecoderFixture, AddManyAddsAllTimeseries) {
  // Arrange
  using AddManyCallbackType = WALEncoder::add_many_generator_callback_type;
  using enum AddManyCallbackType;
  WALEncoder encoder(2, 3);

  const LabelSet ls1{{"metric", "cpu"}};
  const LabelSet ls2{{"metric", "memory"}};
  const LabelSet ls3{{"metric", "disk"}};

  const auto timeseries1 = create_timeseries(ls1, {{1000, 1.0}});
  const auto timeseries2 = create_timeseries(ls2, {{1000, 2.0}});
  const auto timeseries3 = create_timeseries(ls3, {{1000, 3.0}});

  auto generator = [&](auto add_cb) {
    add_cb(timeseries1);
    add_cb(timeseries2);
    add_cb(timeseries3);
  };

  // Act
  const auto state = encoder.add_many<without_hash_value, PromPP::Primitives::Timeseries>(nullptr, 1000, generator);
  WALEncoder::DestroySourceState(state);

  // Assert
  EXPECT_EQ(encoder.buffer().samples_count(), 3U);
  EXPECT_EQ(encoder.buffer().series_count(), 3U);
}

TEST_F(WalEncoderDecoderFixture, AddManyFillsMissingSeriesWithStaleNaN) {
  // Arrange
  using AddManyCallbackType = WALEncoder::add_many_generator_callback_type;
  using enum AddManyCallbackType;
  WALEncoder encoder(2, 3);

  const LabelSet ls1{{"metric", "cpu"}};
  const LabelSet ls2{{"metric", "memory"}};
  const LabelSet ls3{{"metric", "disk"}};

  // first batch: add ls1 and ls2
  auto generator1 = [&](auto add_cb) {
    add_cb(create_timeseries(ls1, {{1000, 1.0}}));
    add_cb(create_timeseries(ls2, {{1000, 2.0}}));
  };

  // second batch: add ls1 and ls3 (ls2 is missing, should get StaleNaN)
  auto generator2 = [&](auto add_cb) {
    add_cb(create_timeseries(ls1, {{2000, 1.5}}));
    add_cb(create_timeseries(ls3, {{2000, 3.0}}));
  };

  // Act
  auto state = encoder.add_many<without_hash_value, PromPP::Primitives::Timeseries>(nullptr, 1000, generator1);
  state = encoder.add_many<without_hash_value, PromPP::Primitives::Timeseries>(state, 2000, generator2);
  decltype(encoder)::DestroySourceState(state);

  // Assert
  const auto samples = collect_samples_from_buffer(encoder.buffer());
  ASSERT_EQ(samples.size(), 5U);

  // ls1: present in both batches
  EXPECT_TRUE(contains_sample(samples, 0, 1000, 1.0));
  EXPECT_TRUE(contains_sample(samples, 0, 2000, 1.5));

  // ls2: present in first batch, StaleNaN in second batch
  EXPECT_TRUE(contains_sample(samples, 1, 1000, 2.0));
  EXPECT_TRUE(contains_stale_nan_sample(samples, 1, 2000));

  // ls3: present only in second batch
  EXPECT_TRUE(contains_sample(samples, 2, 2000, 3.0));
}

class CreateBasicEncoderFromBasicDecoderFixture : public ::testing::Test {
 protected:
  using Encoder = BasicEncoder<EncodingBimap<BareBones::Vector>&>;
  using Decoder = BasicDecoder<EncodingBimap<BareBones::Vector>>;

  struct DecodedPoint {
    Sample sample{};
    uint32_t ls_id{};

    bool operator==(const DecodedPoint&) const noexcept = default;
  };

  const LabelSet kLabelSet1{{"__name__", "test_metric1"}};
  const LabelSet kLabelSet2{{"__name__", "test_metric2"}};

  EncodingBimap<BareBones::Vector> encoder_lss1_;
  Encoder encoder1_{encoder_lss1_, 0, 2};

  EncodingBimap<BareBones::Vector> decoder_lss1_;
  Decoder decoder1_{decoder_lss1_, BasicEncoder<>::version};

  EncodingBimap<BareBones::Vector> decoder_lss2_;
  Decoder decoder2_{decoder_lss2_, BasicEncoder<>::version};

  static auto create_point_decoder(DecodedPoint& point) noexcept {
    return [&point](uint32_t ls_id, int64_t timestamp, double value) { point = DecodedPoint{.sample = Sample(timestamp, value), .ls_id = ls_id}; };
  }

  static void encode_decode_segment(const Sample& sample,
                                    const LabelSet& label_set,
                                    Encoder& encoder,
                                    const std::vector<Decoder*>& decoders,
                                    auto&& segment_handler) {
    std::stringstream stream;

    PromPP::Primitives::Timeseries timeseries;
    timeseries.samples().emplace_back(sample);
    timeseries.label_set().add(label_set);

    encoder.add(timeseries);
    stream << encoder;

    const std::string stream_data = stream.str();
    for (const auto decoder : decoders) {
      EXPECT_NO_THROW(std::ispanstream(std::string_view(stream_data)) >> *decoder);
      EXPECT_NO_THROW(decoder->process_segment(segment_handler));
    }
  };
};

TEST_F(CreateBasicEncoderFromBasicDecoderFixture, Test) {
  // Arrange
  static constexpr auto nop_handler = [](uint32_t, int64_t, double) {};
  static const Sample kThirdSample(3, 3.0);
  static const Sample kFourthSample(3, 3.0);

  encode_decode_segment(Sample(1, 1.0), kLabelSet1, encoder1_, {&decoder1_, &decoder2_}, nop_handler);
  encode_decode_segment(Sample(2, 2.0), kLabelSet1, encoder1_, {&decoder1_, &decoder2_}, nop_handler);

  Encoder encoder2(decoder1_.sample_decoder().gorilla(), decoder_lss1_, decoder1_.shard_id(), decoder1_.pow_two_of_total_shards(),
                   decoder1_.last_processed_segment() + 1, decoder1_.sample_decoder().timestamp_base);

  DecodedPoint third_point{};
  DecodedPoint fourth_point{};

  // Act
  encode_decode_segment(kThirdSample, kLabelSet1, encoder2, {&decoder2_}, create_point_decoder(third_point));
  encode_decode_segment(kFourthSample, kLabelSet2, encoder2, {&decoder2_}, create_point_decoder(fourth_point));

  // Assert
  ASSERT_EQ((DecodedPoint{.sample = kThirdSample, .ls_id = 0}), third_point);
  ASSERT_EQ((DecodedPoint{.sample = kFourthSample, .ls_id = 1}), fourth_point);
}

}  // namespace
