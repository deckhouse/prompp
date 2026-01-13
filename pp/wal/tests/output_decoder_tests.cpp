#include <gtest/gtest.h>

#include "bare_bones/type_traits.h"
#include "primitives/go_model.h"
#include "wal/encoder.h"
#include "wal/hashdex/go_model.h"
#include "wal/output_decoder.h"

struct GoLabelSet {
  PromPP::Primitives::Go::Slice<char> data;
  PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::LabelView> pairs;
};

struct GoTimeSeries {
  GoLabelSet label_set;
  uint64_t timestamp{};
  double value{};

  PROMPP_ALWAYS_INLINE GoTimeSeries() = default;
  PROMPP_ALWAYS_INLINE GoTimeSeries(std::initializer_list<PromPP::Primitives::LabelView> lvs, const PromPP::Primitives::Sample& sample) {
    size_t index{0};
    for (const auto& [ln, lv] : lvs) {
      PromPP::Primitives::Go::LabelView go_label_view;
      label_set.data.push_back(ln.begin(), ln.end());
      label_set.data.push_back(':');
      go_label_view.name = {static_cast<uint32_t>(index), static_cast<uint32_t>(ln.size())};
      index += ln.size() + 1;
      label_set.data.push_back(lv.begin(), lv.end());
      label_set.data.push_back(';');
      go_label_view.value = {static_cast<uint32_t>(index), static_cast<uint32_t>(lv.size())};
      index += lv.size() + 1;
      label_set.pairs.push_back(go_label_view);
    }
    timestamp = static_cast<uint64_t>(sample.timestamp());
    value = sample.value();
  }
};

template <>
struct BareBones::IsTriviallyReallocatable<GoTimeSeries> : std::true_type {};

namespace {

using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::Primitives::Go::Slice;
using PromPP::Primitives::Go::SliceView;
using PromPP::Primitives::Go::String;
using PromPP::Primitives::Go::TimeSeries;
using PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap;
using PromPP::Prometheus::Relabel::rAction;
using PromPP::Prometheus::Relabel::StatelessRelabeler;
using PromPP::WAL::Encoder;
using PromPP::WAL::GoMessage;
using PromPP::WAL::OutputDecoder;
using PromPP::WAL::ProtobufEncoder;
using PromPP::WAL::ProtobufEncoderOld;
using PromPP::WAL::ProtobufEncoderStats;
using PromPP::WAL::RefSample;
using PromPP::WAL::SegmentSamplesStorage;
using PromPP::WAL::ShardRefSample;

using std::operator""sv;

struct EncodeStatistic {
  uint32_t samples;
  uint32_t series;
  int64_t earliest_timestamp;
  int64_t latest_timestamp;
  uint32_t remainder_size;
};

struct RelabelConfigTest {
  std::vector<std::string_view> source_labels{};
  std::string_view separator{};
  std::string_view regex{};
  uint64_t modulus{0};
  std::string_view target_label{};
  std::string_view replacement{};
  uint8_t action{0};
};

struct TestWALOutputDecoder : public testing::Test {
  // external_labels
  std::vector<std::pair<String, String>> vector_external_labels_;
  SliceView<std::pair<String, String>> external_labels_;

  // Encoder
  EncodeStatistic stats_;

  // StatelessRelabeler
  std::vector<RelabelConfigTest> rcts_buf_{{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = ".*", .action = 2}};  // Keep
  std::vector<RelabelConfigTest*> rcts_{&rcts_buf_[0]};
  StatelessRelabeler sr_{rcts_};

  // Output LSS
  EncodingBimap<BareBones::Vector> output_lss_;

  void SetUp() final {
    // external_labels
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size(), vector_external_labels_.size());
  }

  template <class SegmentStream>
  PROMPP_ALWAYS_INLINE void make_segment(const std::vector<GoTimeSeries>& gtss, SegmentStream& segment_stream, Encoder& enc) {
    // make Hashdex
    Slice<GoTimeSeries> go_time_series_slice;
    for (const GoTimeSeries& gts : gtss) {
      go_time_series_slice.push_back(gts);
    }
    const auto go_time_series_slice_view = reinterpret_cast<SliceView<TimeSeries>*>(&go_time_series_slice);
    PromPP::WAL::hashdex::GoModel hx;
    hx.presharding(*go_time_series_slice_view);

    // make Segment from encoder
    enc.add(hx, &stats_);
    enc.finalize(&stats_, segment_stream);
  }

  PROMPP_ALWAYS_INLINE void stateless_relabeler_reset_to(std::initializer_list<RelabelConfigTest> cfgs) {
    rcts_.resize(0);
    rcts_buf_.resize(0);
    rcts_.reserve(cfgs.size());
    rcts_buf_.reserve(cfgs.size());
    for (const RelabelConfigTest& cfg : cfgs) {
      rcts_buf_.push_back(cfg);
    }
    for (RelabelConfigTest& cfg : rcts_buf_) {
      rcts_.push_back(&cfg);
    }

    sr_.reset_to(rcts_);
  }
};

TEST_F(TestWALOutputDecoder, DumpEmptyData) {
  std::stringstream dump;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  wod.dump_to(dump);

  EXPECT_EQ(0, dump.str().size());
}

TEST_F(TestWALOutputDecoder, DumpLoadSingleData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
  segment_stream >> wod;
  wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});

  wod.dump_to(dump);

  EncodingBimap<BareBones::Vector> output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(1, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, DumpLoadDoubleData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  {
    make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  EncodingBimap<BareBones::Vector> output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(2, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, DumpLoadDataEmptyData) {
  std::stringstream dump;
  std::stringstream segment_stream;
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  Encoder enc{uint16_t{0}, uint8_t{0}};

  {
    make_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}}, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  wod.dump_to(dump);

  {
    segment_stream.str("");
    make_segment({{{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id [[maybe_unused]], Timestamp ts [[maybe_unused]], Sample::value_type v [[maybe_unused]]) {});
    wod.dump_to(dump);
  }

  EncodingBimap<BareBones::Vector> output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);

  EXPECT_EQ(2, wod.cache().size());
  EXPECT_EQ(wod.cache(), wod2.cache());
  EXPECT_EQ(output_lss_.size(), output_lss2.size());
  for (size_t i = 0; i < output_lss_.size(); ++i) {
    EXPECT_EQ(output_lss_[i], output_lss2[i]);
  }
}

TEST_F(TestWALOutputDecoder, ProcessSegment) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  const std::vector<std::vector<GoTimeSeries>> incoming_segments{{{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},  // keep
                                                                 {},                                                     // load empty data
                                                                 {
                                                                     {{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}},  // keep
                                                                     {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}},  // keep
                                                                 }};
  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(3);
  for (const auto& segment : incoming_segments) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
  }

  const std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithDrop) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  const std::vector<std::vector<GoTimeSeries>> incoming_segments{
      {{{{"__name__", "value1"}, {"job", "abc1"}}, {10, 1}}},  // drop
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}},   // keep
  };

  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(1);
  size_t processed{0};
  for (const auto& incoming_segment : incoming_segments) {
    segment_stream.str("");
    make_segment(incoming_segment, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) {
      actual_ref_samples.emplace_back(ls_id, ts, v);
      ++processed;
    });
  }

  const std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);
  EXPECT_EQ(1, processed);
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithDump) {
  stateless_relabeler_reset_to({{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2}});  // Keep
  OutputDecoder wod(sr_, output_lss_, external_labels_);
  std::stringstream segment_stream;
  std::stringstream dump;
  Encoder enc{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> incoming_segments{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}}};

  std::vector<RefSample> actual_ref_samples;
  actual_ref_samples.reserve(3);
  for (const auto& segment : incoming_segments) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc);
    segment_stream >> wod;
    wod.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
    wod.dump_to(dump);
  }

  std::vector<RefSample> expected_ref_samples{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1}};
  EXPECT_EQ(expected_ref_samples, actual_ref_samples);

  EncodingBimap<BareBones::Vector> output_lss2;
  OutputDecoder wod2(sr_, output_lss2, external_labels_);
  wod2.load_from(dump);
  Encoder enc2{uint16_t{0}, uint8_t{0}};

  std::vector<std::vector<GoTimeSeries>> incoming_segments_2{
      {{{{"__name__", "value1"}, {"job", "abc"}}, {10, 1}}},
      {},  // load empty data
      {{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}, {{{"__name__", "value2"}, {"job", "abc"}}, {11, 1}}},
      {{{{"__name__", "value1"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value2"}, {"job", "abc"}}, {12, 1}},
       {{{"__name__", "value3"}, {"job", "abc"}}, {12, 1}}}};

  actual_ref_samples.clear();
  actual_ref_samples.reserve(6);
  for (const auto& segment : incoming_segments_2) {
    segment_stream.str("");
    make_segment(segment, segment_stream, enc2);
    segment_stream >> wod2;
    wod2.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_ref_samples.emplace_back(ls_id, ts, v); });
  }

  std::vector<RefSample> expected_ref_samples_2{{.id = 0, .t = 10, .v = 1}, {.id = 0, .t = 11, .v = 1}, {.id = 1, .t = 11, .v = 1},
                                                {.id = 0, .t = 12, .v = 1}, {.id = 1, .t = 12, .v = 1}, {.id = 2, .t = 12, .v = 1}};
  EXPECT_EQ(expected_ref_samples_2, actual_ref_samples);
}

TEST_F(TestWALOutputDecoder, ProcessSegmentWithLabelDrop) {
  // Arrange
  stateless_relabeler_reset_to({
      {.source_labels = std::vector{"job"sv}, .regex = "abc", .action = rAction::rKeep},
      {.regex = "resource", .action = rAction::rLabelDrop},
  });
  Encoder encoder{uint16_t{}, uint8_t{}};
  OutputDecoder decoder(sr_, output_lss_, external_labels_);
  std::vector<RefSample> actual_samples;

  // Act
  const auto encode_decode_segment = [&](const std::vector<GoTimeSeries>& incoming_segment) {
    std::stringstream segment_stream;
    make_segment(incoming_segment, segment_stream, encoder);
    segment_stream >> decoder;
    decoder.process_segment([&](LabelSetID ls_id, Timestamp ts, Sample::value_type v) { actual_samples.emplace_back(ls_id, ts, v); });
  };

  encode_decode_segment({{{{"__name__", "value1"}, {"job", "abc"}}, {11, 1}}});                       // keep
  encode_decode_segment({{{{"__name__", "value1"}, {"job", "abc"}, {"resource", "cpu"}}, {11, 2}}});  // labeldrop

  // Assert
  EXPECT_EQ((std::vector<RefSample>{{.id = 0, .t = 11, .v = 1}, {.id = 0, .t = 11, .v = 2}}), actual_samples);
}

//
// ProtobufEncoderOld
//

struct TestProtobufEncoderOld : public testing::Test {};

TEST_F(TestProtobufEncoderOld, Encode) {
  // Arrange
  EncodingBimap<BareBones::Vector> output_lss0;
  EncodingBimap<BareBones::Vector> output_lss1;

  std::vector<RefSample> ref_samples0{{.id = output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value1"}, {"job", "abc"}}), .t = 10, .v = 1},
                                      {.id = output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value1"}, {"job", "abc"}}), .t = 9, .v = 2},
                                      {.id = output_lss0.find_or_emplace(LabelViewSet{{"__name__", "value2"}, {"job", "abc"}}), .t = 10, .v = 1}};
  ShardRefSample srs0;
  srs0.ref_samples.reset_to(ref_samples0.data(), ref_samples0.size(), ref_samples0.size());
  srs0.shard_id = 0;

  std::vector<RefSample> ref_samples1{{.id = output_lss1.find_or_emplace(LabelViewSet{{"__name__", "value3"}, {"job", "abc3"}}), .t = 10, .v = 1}};
  ShardRefSample srs1;
  srs1.ref_samples.reset_to(ref_samples1.data(), ref_samples1.size(), ref_samples1.size());
  srs1.shard_id = 1;

  std::vector vector_batch{&srs0, &srs1};
  SliceView<ShardRefSample*> batch;
  batch.reset_to(vector_batch.data(), vector_batch.size(), vector_batch.size());

  ProtobufEncoderOld penc(std::move(std::vector{&output_lss0, &output_lss1}));
  Slice<Slice<char>> out_slices;
  out_slices.resize(2);
  Slice<ProtobufEncoderStats> stats;
  stats.resize(2);

  // Act
  penc.encode(batch, out_slices, stats);

  // Assert
  std::string proto1;
  EXPECT_TRUE(snappy::Uncompress(out_slices[0].data(), out_slices[0].size(), &proto1));
  EXPECT_TRUE(std::ranges::equal(
      std::array{10,  58, 10,  18, 10, 8,   95,  95, 110, 97, 109, 101, 95, 95, 18,  6,  118, 97,  108, 117, 101, 49, 10,  10,  10,  3,   106,
                 111, 98, 18,  3,  97, 98,  99,  18, 11,  9,  0,   0,   0,  0,  0,   0,  0,   64,  16,  9,   18,  11, 9,   0,   0,   0,   0,
                 0,   0,  -16, 63, 16, 10,  10,  46, 10,  18, 10,  8,   95, 95, 110, 97, 109, 101, 95,  95,  18,  6,  118, 97,  108, 117, 101,
                 51,  10, 11,  10, 3,  106, 111, 98, 18,  4,  97,  98,  99, 51, 18,  11, 9,   0,   0,   0,   0,   0,  0,   -16, 63,  16,  10},
      std::span(reinterpret_cast<int8_t*>(proto1.data()), proto1.size())));
  EXPECT_EQ(10, stats[0].max_timestamp);
  EXPECT_EQ(3, stats[0].samples_count);

  std::string proto2;
  EXPECT_TRUE(snappy::Uncompress(out_slices[1].data(), out_slices[1].size(), &proto2));
  EXPECT_TRUE(std::ranges::equal(std::array{10, 45, 10,  18,  10, 8,  95, 95, 110, 97, 109, 101, 95, 95, 18, 6, 118, 97, 108, 117, 101, 50, 10, 10,
                                            10, 3,  106, 111, 98, 18, 3,  97, 98,  99, 18,  11,  9,  0,  0,  0, 0,   0,  0,   -16, 63,  16, 10},
                                 std::span(reinterpret_cast<int8_t*>(proto2.data()), proto2.size())));
  EXPECT_EQ(10, stats[1].max_timestamp);
  EXPECT_EQ(1, stats[1].samples_count);
}

class ProtobufEncoderFixture : public testing::Test {
 protected:
  ProtobufEncoder<EncodingBimap<BareBones::Vector>> encoder_;
};

TEST_F(ProtobufEncoderFixture, Test) {
  // Arrange
  std::vector<EncodingBimap<BareBones::Vector>> lss_list(2);
  lss_list[0].find_or_emplace(LabelViewSet{{"__name__", "value1"}, {"job", "abc"}});
  lss_list[0].find_or_emplace(LabelViewSet{{"__name__", "value2"}, {"job", "abc"}});
  lss_list[1].find_or_emplace(LabelViewSet{{"__name__", "value3"}, {"job", "abc3"}});

  const auto lss_getter = [&lss_list](uint32_t id) -> const EncodingBimap<BareBones::Vector>& { return lss_list[id]; };

  std::vector<SegmentSamplesStorage> storages_list(2);
  storages_list[0].add(0, Sample(10, 1.0));
  storages_list[0].add(0, Sample(9, 2));
  storages_list[0].add(1, Sample(10, 1));
  storages_list[1].add(0, Sample(10, 1));

  std::vector<GoMessage> messages(2);
  std::string proto;

  // Act
  encoder_.encode(std::span(storages_list.data(), storages_list.size()), lss_getter, 0, 2, messages[0]);
  encoder_.encode(std::span(storages_list.data(), storages_list.size()), lss_getter, 1, 2, messages[1]);

  // Assert
  EXPECT_EQ(3U, messages[0].samples_count);
  EXPECT_EQ(10, messages[0].max_timestamp);
  EXPECT_TRUE(snappy::Uncompress(messages[0].buffer.data(), messages[0].buffer.size(), &proto));
  EXPECT_TRUE(std::ranges::equal(
      std::array{0x0A, 0x3A, 0x0A, 0x12, 0x0A, 0x08, 0x5F, 0x5F, 0x6E, 0x61, 0x6D, 0x65, 0x5F, 0x5F, 0x12, 0x06, 0x76, 0x61, 0x6C, 0x75, 0x65, 0x31,
                 0x0A, 0x0A, 0x0A, 0x03, 0x6A, 0x6F, 0x62, 0x12, 0x03, 0x61, 0x62, 0x63, 0x12, 0x0B, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
                 0x40, 0x10, 0x09, 0x12, 0x0B, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F, 0x10, 0x0A, 0x0A, 0x2E, 0x0A, 0x12, 0x0A, 0x08,
                 0x5F, 0x5F, 0x6E, 0x61, 0x6D, 0x65, 0x5F, 0x5F, 0x12, 0x06, 0x76, 0x61, 0x6C, 0x75, 0x65, 0x33, 0x0A, 0x0B, 0x0A, 0x03, 0x6A, 0x6F,
                 0x62, 0x12, 0x04, 0x61, 0x62, 0x63, 0x33, 0x12, 0x0B, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F, 0x10, 0x0A},
      std::span(reinterpret_cast<uint8_t*>(proto.data()), proto.size())));

  EXPECT_EQ(1U, messages[1].samples_count);
  EXPECT_EQ(10, messages[1].max_timestamp);
  EXPECT_TRUE(snappy::Uncompress(messages[1].buffer.data(), messages[1].buffer.size(), &proto));
  EXPECT_TRUE(std::ranges::equal(
      std::array{0x0A, 0x2D, 0x0A, 0x12, 0x0A, 0x08, 0x5F, 0x5F, 0x6E, 0x61, 0x6D, 0x65, 0x5F, 0x5F, 0x12, 0x06, 0x76, 0x61, 0x6C, 0x75, 0x65, 0x32, 0x0A, 0x0A,
                 0x0A, 0x03, 0x6A, 0x6F, 0x62, 0x12, 0x03, 0x61, 0x62, 0x63, 0x12, 0x0B, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F, 0x10, 0x0A},
      std::span(reinterpret_cast<uint8_t*>(proto.data()), proto.size())));
}

}  // namespace
