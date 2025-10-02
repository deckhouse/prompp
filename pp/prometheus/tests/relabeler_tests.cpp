#include <gtest/gtest.h>
#include <cstdint>
#include <initializer_list>
#include <string>
#include <string_view>
#include <vector>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "prometheus/relabeler.h"

namespace {

using PromPP::Primitives::Label;
using PromPP::Primitives::LabelsBuilder;
using PromPP::Primitives::LabelSet;
using PromPP::Primitives::LabelView;
using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Sample;
using PromPP::Primitives::Timestamp;
using PromPP::Primitives::Go::SliceView;
using PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap;
using PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap;
using PromPP::Prometheus::Relabel::hard_validate;
using PromPP::Prometheus::Relabel::InnerSerie;
using PromPP::Prometheus::Relabel::InnerSeries;
using PromPP::Prometheus::Relabel::MetricLimits;
using PromPP::Prometheus::Relabel::PerGoroutineRelabeler;
using PromPP::Prometheus::Relabel::PerShardRelabeler;
using PromPP::Prometheus::Relabel::RelabelerStateUpdate;
using PromPP::Prometheus::Relabel::relabelStatus;
using PromPP::Prometheus::Relabel::StaleNaNsState;
using PromPP::Prometheus::Relabel::StatelessRelabeler;
using enum PromPP::Prometheus::Relabel::rAction;
using enum relabelStatus;

using GoString = PromPP::Primitives::Go::String;
using PromPP::Primitives::kNullTimestamp;
using PromPP::Prometheus::kStaleNan;

using GoLabel = std::pair<GoString, GoString>;

struct RelabelConfig {
  std::vector<std::string_view> source_labels{};
  std::string_view separator{};
  std::string_view regex{};
  uint64_t modulus{0};
  std::string_view target_label{};
  std::string_view replacement{};
  uint8_t action{0};
};

class ItemTest {
 public:
  ItemTest(LabelViewSet&& label_set, std::vector<Sample>&& samples)
      : label_set_(std::move(label_set)), samples_(std::move(samples)), hash_(hash_value(label_set_)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

  template <class Timeseries>
  PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
    timeseries.label_set().add(label_set_);
    for (const auto& sample : samples_) {
      timeseries.samples().emplace_back(sample.timestamp(), sample.value());
    }
  }

 private:
  LabelViewSet label_set_;
  std::vector<Sample> samples_;
  size_t hash_;
};

class HashdexTest : public std::vector<ItemTest> {
  using Base = std::vector<ItemTest>;

 public:
  using Base::Base;

  void emplace_back(LabelViewSet&& label_set, std::vector<Sample>&& samples) { Base::emplace_back(std::move(label_set), std::move(samples)); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& metrics() const noexcept { return *this; }
  [[nodiscard]] static PROMPP_ALWAYS_INLINE auto metadata() noexcept {
    struct Stub {};
    return Stub{};
  }
};

static_assert(PromPP::Prometheus::hashdex::HashdexInterface<HashdexTest>);

struct HardValidateCase {
  LabelViewSet labels;
  std::optional<MetricLimits> limits;
  relabelStatus expected;
};

class HardValidateFixture : public testing::TestWithParam<HardValidateCase> {
 protected:
  LabelsBuilder builder_;
  relabelStatus rstatus_{rsKeep};
};

TEST_P(HardValidateFixture, Test) {
  // Arrange
  builder_.reset(GetParam().labels);

  // Act
  hard_validate(rstatus_, builder_, GetParam().limits ? &GetParam().limits.value() : nullptr);

  // Assert
  EXPECT_EQ(GetParam().expected, rstatus_);
}

INSTANTIATE_TEST_SUITE_P(Valid, HardValidateFixture, testing::Values(HardValidateCase{.labels = {{"__name__", "value"}, {"job", "abc"}}, .expected = rsKeep}));
INSTANTIATE_TEST_SUITE_P(Invalid,
                         HardValidateFixture,
                         testing::Values(HardValidateCase{.labels = {{"__value__", "value"}, {"job", "abc"}}, .expected = rsInvalid}));
INSTANTIATE_TEST_SUITE_P(
    NoLimit,
    HardValidateFixture,
    testing::Values(HardValidateCase{.labels = {{"__name__", "value"}, {"job", "abc"}, {"jub", "buj"}}, .limits = MetricLimits{}, .expected = rsKeep}));
INSTANTIATE_TEST_SUITE_P(LabelCountLimitExceeded,
                         HardValidateFixture,
                         testing::Values(HardValidateCase{.labels = {{"__name__", "value"}, {"job", "abc"}, {"jub", "buj"}},
                                                          .limits = MetricLimits{.label_limit = 2},
                                                          .expected = rsInvalid}));
INSTANTIATE_TEST_SUITE_P(LabelNameLengthLimitExceeded,
                         HardValidateFixture,
                         testing::Values(HardValidateCase{.labels = {{"__name__", "value"}, {"job", "abc"}, {"jub", "buj"}},
                                                          .limits = MetricLimits{.label_name_length_limit = 3},
                                                          .expected = rsInvalid}));
INSTANTIATE_TEST_SUITE_P(LabelValueLengthLimitExceeded,
                         HardValidateFixture,
                         testing::Values(HardValidateCase{.labels = {{"__name__", "value"}, {"job", "abc"}, {"jub", "buj"}},
                                                          .limits = MetricLimits{.label_value_length_limit = 3},
                                                          .expected = rsInvalid}));

struct Stats {
  uint32_t samples_added{0};
  uint32_t series_added{0};
  uint32_t series_drop{0};

  bool operator==(const Stats& other) const noexcept = default;
};

class PerGoroutineRelabelerFixture : public testing::Test {
 protected:
  static constexpr uint16_t kNumberOfShards = 2;

  std::vector<std::unique_ptr<InnerSeries>> vector_shards_inner_series_;
  SliceView<InnerSeries*> shards_inner_series_{};

  std::vector<std::unique_ptr<PromPP::Prometheus::Relabel::RelabeledSeries>> vector_relabeled_results_;
  SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> relabeled_results_{};

  std::vector<GoLabel> vector_target_labels_{};
  PromPP::Prometheus::Relabel::RelabelerOptions o_;
  Stats stats_;
  HashdexTest hx_;
  EncodingBimap<BareBones::Vector> lss_;
  PromPP::Prometheus::Relabel::Cache cache_{};

  void reset() {
    TearDown();
    SetUp();
  }

  void add_target_labels(const LabelViewSet& target_labels) {
    vector_target_labels_.resize(target_labels.size());
    for (size_t i = 0; i < vector_target_labels_.size(); i++) {
      vector_target_labels_[i].first.reset_to(target_labels[i].first.data(), target_labels[i].first.size());
      vector_target_labels_[i].second.reset_to(target_labels[i].second.data(), target_labels[i].second.size());
    }
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size(), vector_target_labels_.size());
  }

  void SetUp() final {
    vector_shards_inner_series_.emplace_back(std::make_unique<InnerSeries>());
    vector_shards_inner_series_.emplace_back(std::make_unique<InnerSeries>());
    shards_inner_series_.reset_to(reinterpret_cast<InnerSeries**>(vector_shards_inner_series_.data()), vector_shards_inner_series_.size(),
                                  vector_shards_inner_series_.size());

    vector_relabeled_results_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::RelabeledSeries>());
    vector_relabeled_results_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::RelabeledSeries>());
    relabeled_results_.reset_to(reinterpret_cast<PromPP::Prometheus::Relabel::RelabeledSeries**>(vector_relabeled_results_.data()),
                                vector_shards_inner_series_.size(), vector_shards_inner_series_.size());

    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size(), vector_target_labels_.size());
  }

  void TearDown() final {
    vector_shards_inner_series_.clear();
    vector_relabeled_results_.clear();
  }
};

TEST_F(PerGoroutineRelabelerFixture, KeepOnNotFoundInCache) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
}

TEST_F(PerGoroutineRelabelerFixture, InnerSeriesAlreadyAdded) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
}

TEST_F(PerGoroutineRelabelerFixture, KeepOnFoundInCache) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  shards_inner_series_[1]->clear();
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 2, .series_added = 1}), stats_);
}

TEST_F(PerGoroutineRelabelerFixture, KeepNotEqual) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "no-match", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
  EXPECT_EQ(Stats{.series_drop = 1}, stats_);
}

TEST_F(PerGoroutineRelabelerFixture, KeepEqualThenNotEqual) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  reset();
  hx_ = HashdexTest{{{{"__name__", "value"}, {"job", "abcd"}}, {{1000, 0.1}}}};
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1, .series_drop = 1}), stats_);
}

TEST_F(PerGoroutineRelabelerFixture, ReplaceToNewLS2) {
  // Arrange
  const RelabelConfig config{
      .source_labels = {{"__name__"}}, .separator = ";", .regex = ".*(o).*", .target_label = "replaced", .replacement = "$1", .action = rReplace};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 1}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  ASSERT_EQ(1U, update_data.size());
  EXPECT_EQ((LabelViewSet{{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}}), lss_[update_data[0].relabeled_ls_id]);
}

TEST_F(PerGoroutineRelabelerFixture, ReplaceToNewLS3) {
  // Arrange
  const RelabelConfig config{.separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "blabla", .action = rReplace};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}, {{1000, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  PerShardRelabeler::update_relabeler_state(cache_, &update_data, 1);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 1}}, shards_inner_series_[0]->data()));
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 1);
  EXPECT_EQ((LabelViewSet{{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "blabla"}}), lss_[update_data[0].relabeled_ls_id]);
}

TEST_F(PerGoroutineRelabelerFixture, InputRelabelingWithStalenans_Default) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{kNullTimestamp, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  StaleNaNsState state;
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state, 1000);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, HashdexTest{}, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state,
                                            2000);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 2);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}, {.sample = Sample(2000, kStaleNan), .ls_id = 0}},
                                 shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 0);
}

TEST_F(PerGoroutineRelabelerFixture, InputRelabelingWithStalenans_DefaultHonorTimestamps) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{kNullTimestamp, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  StaleNaNsState state;
  RelabelerStateUpdate update_data;
  o_.honor_timestamps = true;

  // Act
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state, 1000);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, HashdexTest{}, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state,
                                            2000);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 2);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}, {.sample = Sample(2000, kStaleNan), .ls_id = 0}},
                                 shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 0);
}

TEST_F(PerGoroutineRelabelerFixture, InputRelabelingWithStalenans_WithMetricTimestamp) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1712567046855, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  StaleNaNsState state;
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state, 1000);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, HashdexTest{}, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state,
                                            2000);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 2);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 0}, {.sample = Sample(2000, kStaleNan), .ls_id = 0}},
                                 shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 0);
}

TEST_F(PerGoroutineRelabelerFixture, InputRelabelingWithStalenans_HonorTimestamps) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1500, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  o_.honor_timestamps = true;
  StaleNaNsState state;
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state, 1000);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, HashdexTest{}, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state,
                                            2000);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1500, 0.1), .ls_id = 0}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 0);
}

TEST_F(PerGoroutineRelabelerFixture, InputRelabelingWithStalenans_HonorTimestampsAndTrackStaleness) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "value"}, {"job", "abc"}}, {{1500, 0.1}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  o_.honor_timestamps = true;
  o_.track_timestamps_staleness = true;
  StaleNaNsState state;
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state, 1000);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  relabeler.input_relabeling_with_stalenans(lss_, lss_, cache_, HashdexTest{}, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_, state,
                                            2000);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 2);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1500, 0.1), .ls_id = 0}, {.sample = Sample(2000, kStaleNan), .ls_id = 0}},
                                 shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 0);
}

TEST_F(PerGoroutineRelabelerFixture, TargetLabels_HappyPath) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}, {{1000, 0.1}});
  add_target_labels(LabelViewSet{{"a_name", "target_a_value"}, {"z_name", "target_z_value"}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 0};
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  PerShardRelabeler::update_relabeler_state(cache_, &update_data, 1);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 1}}, shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 1);
  EXPECT_EQ((LabelViewSet{{"__name__", "booom"}, {"a_name", "target_a_value"}, {"jab", "baj"}, {"job", "abc"}, {"z_name", "target_z_value"}}),
            lss_[update_data[0].relabeled_ls_id]);
}

TEST_F(PerGoroutineRelabelerFixture, TargetLabels_ExportedLabel) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rsKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}, {{1000, 0.1}});
  add_target_labels(LabelViewSet{{"jab", "target_a_value"}, {"z_name", "target_z_value"}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 0};
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  PerShardRelabeler::update_relabeler_state(cache_, &update_data, 1);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 1}}, shards_inner_series_[0]->data()));
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 1);
  EXPECT_EQ((LabelViewSet{{"__name__", "booom"}, {"exported_jab", "baj"}, {"jab", "target_a_value"}, {"job", "abc"}, {"z_name", "target_z_value"}}),
            lss_[update_data[0].relabeled_ls_id]);
}

TEST_F(PerGoroutineRelabelerFixture, TargetLabels_ExportedLabel_Honor) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rsKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_.emplace_back({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}, {{1000, 0.1}});
  add_target_labels(LabelViewSet{{"jab", "target_a_value"}, {"z_name", "target_z_value"}});
  PerGoroutineRelabeler relabeler{kNumberOfShards, 0};
  o_.honor_labels = true;
  RelabelerStateUpdate update_data;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);
  PerShardRelabeler::append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  PerShardRelabeler::update_relabeler_state(cache_, &update_data, 1);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, 0.1), .ls_id = 1}}, shards_inner_series_[0]->data()));
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
  EXPECT_EQ((Stats{.samples_added = 1, .series_added = 1}), stats_);
  EXPECT_EQ(update_data.size(), 1);
  EXPECT_EQ((LabelViewSet{{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}, {"z_name", "target_z_value"}}), lss_[update_data[0].relabeled_ls_id]);
}

TEST_F(PerGoroutineRelabelerFixture, SampleLimitExceeded) {
  // Arrange
  const RelabelConfig config{.source_labels = {{"job"}}, .regex = "abc", .action = rKeep};
  const StatelessRelabeler stateless_relabeler(std::initializer_list{&config});
  hx_ = HashdexTest{
      {{{"__name__", "value"}, {"job", "abc"}}, {{1000, kStaleNan}}},
      {{{"__name__", "value"}, {"job", "abc"}}, {{2000, 0.1}}},
      {{{"__name__", "value"}, {"job", "abc"}}, {{3000, 0.1}}},
  };
  MetricLimits limits{.sample_limit = 1};
  PerGoroutineRelabeler relabeler{kNumberOfShards, 1};
  o_.metric_limits = &limits;

  // Act
  relabeler.input_relabeling(lss_, lss_, cache_, hx_, o_, stateless_relabeler, stats_, shards_inner_series_, relabeled_results_);

  // Assert
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_TRUE(std::ranges::equal(std::vector<InnerSerie>{{.sample = Sample(1000, kStaleNan), .ls_id = 0}, {.sample = Sample(2000, 0.1), .ls_id = 0}},
                                 shards_inner_series_[1]->data()));
  EXPECT_EQ((Stats{.samples_added = 2, .series_added = 1}), stats_);
}

class TargetLabelsFixture : public testing::Test {
 protected:
  static constexpr uint16_t kNumberOfShards = 2;

  std::vector<GoLabel> vector_target_labels_;

  PromPP::Prometheus::Relabel::RelabelerOptions o_;
  std::vector<RelabelConfig*> rcts_;
  std::vector<GoLabel> vector_external_labels_;
  SliceView<GoLabel> external_labels_{};

  LabelsBuilder builder_;

  StatelessRelabeler stateless_relabeler_{rcts_};
  PerShardRelabeler relabeler_{external_labels_, &stateless_relabeler_, kNumberOfShards, 1};

  void SetUp() final {
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size(), vector_target_labels_.size());
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size(), vector_external_labels_.size());
  }

  void add_target_labels(const LabelViewSet& list_target_labels) {
    vector_target_labels_.resize(list_target_labels.size());
    for (size_t i = 0; i < vector_target_labels_.size(); i++) {
      vector_target_labels_[i].first.reset_to(list_target_labels[i].first.data(), list_target_labels[i].first.size());
      vector_target_labels_[i].second.reset_to(list_target_labels[i].second.data(), list_target_labels[i].second.size());
    }
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size(), vector_target_labels_.size());
  }
};

TEST_F(TargetLabelsFixture, ResolveConflictingExposedLabels_EmptyConflictingLabels) {
  // Arrange
  const LabelViewSet labels{{"c_name", "c_value"}};

  builder_.reset(labels);

  std::vector<Label> conflicting_exposed_labels{};

  // Act
  relabeler_.resolve_conflicting_exposed_labels(builder_, conflicting_exposed_labels);

  // Assert
  EXPECT_EQ(labels, builder_.label_view_set());
  EXPECT_EQ(labels, builder_.label_set());
}

TEST_F(TargetLabelsFixture, ResolveConflictingExposedLabels) {
  // Arrange
  builder_.reset(LabelViewSet{{"c_name", "c_value"}});

  std::vector<Label> conflicting_exposed_labels{{"c_name", "a_value"}};
  const LabelViewSet expected_labels{{"c_name", "c_value"}, {"exported_c_name", "a_value"}};

  // Act
  relabeler_.resolve_conflicting_exposed_labels(builder_, conflicting_exposed_labels);

  // Assert
  EXPECT_EQ(expected_labels, builder_.label_view_set());
  EXPECT_EQ(expected_labels, builder_.label_set());
}

TEST_F(TargetLabelsFixture, ResolveConflictingExposedLabels_ExportedLabel) {
  // Arrange
  builder_.reset(LabelViewSet{{"c_name", "c_value"}});

  std::vector<Label> conflicting_exposed_labels{{"exported_c_name", "a_value"}};
  const LabelViewSet expected_labels{{"c_name", "c_value"}, {"exported_exported_c_name", "a_value"}};

  // Act
  relabeler_.resolve_conflicting_exposed_labels(builder_, conflicting_exposed_labels);

  // Assert
  EXPECT_EQ(expected_labels, builder_.label_view_set());
  EXPECT_EQ(expected_labels, builder_.label_set());
}

TEST_F(TargetLabelsFixture, InjectTargetLabels_EmptyLabels) {
  // Arrange
  const LabelViewSet labels{{"c_name", "c_value"}};

  builder_.reset(labels);

  // Act
  const auto changed = relabeler_.inject_target_labels(builder_, o_);

  // Assert
  EXPECT_FALSE(changed);
  EXPECT_EQ(labels, builder_.label_view_set());
}

TEST_F(TargetLabelsFixture, InjectTargetLabels_HappyPath) {
  // Arrange
  builder_.reset(LabelViewSet{{"c_name", "c_value"}});
  add_target_labels(LabelViewSet{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}});

  // Act
  const auto changed = relabeler_.inject_target_labels(builder_, o_);

  // Assert
  EXPECT_TRUE(changed);
  EXPECT_EQ((LabelViewSet{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}}), builder_.label_view_set());
}

TEST_F(TargetLabelsFixture, InjectTargetLabels_ConflictingLabels) {
  // Arrange
  builder_.reset(LabelViewSet{{"a_name", "a_value"}, {"c_name", "c_value"}});

  add_target_labels(LabelViewSet{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}});

  // Act
  const auto changed = relabeler_.inject_target_labels(builder_, o_);

  // Assert
  EXPECT_TRUE(changed);
  EXPECT_EQ((LabelViewSet{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}, {"exported_a_name", "a_value"}}),
            builder_.label_view_set());
}

TEST_F(TargetLabelsFixture, InjectTargetLabels_ConflictingLabels_ExportedLabel) {
  // Arrange
  builder_.reset(LabelViewSet{{"a_name", "a_value"}, {"c_name", "c_value"}});

  add_target_labels(LabelViewSet{{"a_name", "target_a_value"}, {"exported_a_name", "exported_target_a_value"}});
  LabelViewSet expected_labels{
      {"a_name", "target_a_value"}, {"c_name", "c_value"}, {"exported_a_name", "exported_target_a_value"}, {"exported_exported_a_name", "a_value"}};

  // Act
  const auto changed = relabeler_.inject_target_labels(builder_, o_);

  // Assert
  EXPECT_TRUE(changed);
  EXPECT_EQ((LabelViewSet{
                {"a_name", "target_a_value"}, {"c_name", "c_value"}, {"exported_a_name", "exported_target_a_value"}, {"exported_exported_a_name", "a_value"}}),
            builder_.label_view_set());
}

TEST_F(TargetLabelsFixture, InjectTargetLabels_ConflictingLabels_Honor) {
  // Arrange
  builder_.reset(LabelViewSet{{"a_name", "a_value"}, {"c_name", "c_value"}});

  add_target_labels(LabelViewSet{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}});
  o_.honor_labels = true;

  // Act
  const auto changed = relabeler_.inject_target_labels(builder_, o_);

  // Assert
  EXPECT_TRUE(changed);
  EXPECT_EQ((LabelViewSet{{"a_name", "a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}}), builder_.label_view_set());
}

struct ValidateCase {
  std::string_view value;
  bool expected;
};

class LabelsNameIsValidFixture : public testing::TestWithParam<ValidateCase> {};

TEST_P(LabelsNameIsValidFixture, Test) {
  // Arrange

  // Act
  const auto result = PromPP::Prometheus::Relabel::label_name_is_valid(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Valid,
                         LabelsNameIsValidFixture,
                         testing::Values(ValidateCase{.value = "Avalid_23name", .expected = true},
                                         ValidateCase{.value = "_Avalid_23name", .expected = true},
                                         ValidateCase{.value = "avalid_23name", .expected = true}));

INSTANTIATE_TEST_SUITE_P(Invalid,
                         LabelsNameIsValidFixture,
                         testing::Values(ValidateCase{.value = "", .expected = false},
                                         ValidateCase{.value = "1valid_23name", .expected = false},
                                         ValidateCase{.value = "Ava:lid_23name", .expected = false},
                                         ValidateCase{.value = "a lid_23name", .expected = false},
                                         ValidateCase{.value = ":leading_colon", .expected = false},
                                         ValidateCase{.value = "colon:in:the:middle", .expected = false},
                                         ValidateCase{.value = "a\xc5z", .expected = false}));

class MetricNameIsValidFixture : public testing::TestWithParam<ValidateCase> {};

TEST_P(MetricNameIsValidFixture, Test) {
  // Arrange

  // Act
  const auto result = PromPP::Prometheus::Relabel::metric_name_value_is_valid(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Valid,
                         MetricNameIsValidFixture,
                         testing::Values(ValidateCase{.value = "Avalid_23name", .expected = true},
                                         ValidateCase{.value = "_Avalid_23name", .expected = true},
                                         ValidateCase{.value = "avalid_23name", .expected = true},
                                         ValidateCase{.value = "Ava:lid_23name", .expected = true},
                                         ValidateCase{.value = ":leading_colon", .expected = true},
                                         ValidateCase{.value = "colon:in:the:middle", .expected = true}));

INSTANTIATE_TEST_SUITE_P(Invalid,
                         MetricNameIsValidFixture,
                         testing::Values(ValidateCase{.value = "", .expected = false},
                                         ValidateCase{.value = "1valid_23name", .expected = false},
                                         ValidateCase{.value = "a lid_23name", .expected = false},
                                         ValidateCase{.value = "a\xc5z", .expected = false}));

class LabelValueIsValidFixture : public testing::TestWithParam<ValidateCase> {};

TEST_P(LabelValueIsValidFixture, Test) {
  // Arrange

  // Act
  const auto result = PromPP::Prometheus::Relabel::label_value_is_valid(GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Valid,
                         LabelValueIsValidFixture,
                         testing::Values(ValidateCase{.value = "Avalid_23name", .expected = true},
                                         ValidateCase{.value = "_Avalid_23name", .expected = true},
                                         ValidateCase{.value = "avalid_23name", .expected = true},
                                         ValidateCase{.value = "", .expected = true},
                                         ValidateCase{.value = "1valid_23name", .expected = true},
                                         ValidateCase{.value = "Ava:lid_23name", .expected = true},
                                         ValidateCase{.value = "a lid_23name", .expected = true},
                                         ValidateCase{.value = ":leading_colon", .expected = true},
                                         ValidateCase{.value = "colon:in:the:middle", .expected = true},
                                         ValidateCase{.value = "ol\xc3\xa1 mundo", .expected = true},
                                         ValidateCase{.value = "\xe4\xbd\xa0\xe5\xa5\xbd\xe4\xb8\x96\xe7\x95\x8c", .expected = true},
                                         ValidateCase{.value = "\x7e", .expected = true}));

INSTANTIATE_TEST_SUITE_P(Invalid,
                         LabelValueIsValidFixture,
                         testing::Values(ValidateCase{.value = "\xa0\xa1", .expected = false},
                                         ValidateCase{.value = "a\xc5z", .expected = false},
                                         ValidateCase{.value = "\x80\x8F\x90\x9FzxcasdAA:", .expected = false}));

class StaleNaNsStateFixture : public testing::Test {};

TEST_F(StaleNaNsStateFixture, Swap) {
  // Arrange
  static constexpr uint32_t kCurrentLsId = 42;

  StaleNaNsState result;
  result.add_input(kCurrentLsId);

  std::vector<uint32_t> input_ls_ids;
  std::vector<uint32_t> target_ls_ids;

  auto get_callback = [&](std::vector<uint32_t>& ids) -> auto { return [&ids](uint32_t ls_id) { ids.push_back(ls_id); }; };

  // Act
  result.swap(get_callback(input_ls_ids), get_callback(target_ls_ids));
  result.swap(get_callback(input_ls_ids), get_callback(target_ls_ids));

  // Assert
  EXPECT_EQ(std::vector<uint32_t>{42}, input_ls_ids);
  EXPECT_EQ(std::vector<uint32_t>{}, target_ls_ids);
}

}  // namespace
