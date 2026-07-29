#include <gtest/gtest.h>
#include <initializer_list>

#include "primitives/labels_builder.h"
#include "prometheus/stateless_relabeler.h"

namespace {

using PromPP::Primitives::LabelsBuilder;
using PromPP::Primitives::LabelView;
using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::Relabel::PatternPart;
using PromPP::Prometheus::Relabel::process_external_labels;
using PromPP::Prometheus::Relabel::Regexp;
using PromPP::Prometheus::Relabel::RelabelConfig;
using PromPP::Prometheus::Relabel::relabelStatus;
using PromPP::Prometheus::Relabel::StatelessRelabeler;

using enum relabelStatus;
using enum PromPP::Prometheus::Relabel::rAction;

struct GoRelabelConfig {
  std::vector<std::string_view> source_labels{};
  std::string_view separator{};
  std::string_view regex{};
  uint64_t modulus{0};
  std::string_view target_label{};
  std::string_view replacement{};
  uint8_t action{0};
};

class PatternPartFixture : public testing::Test {
 protected:
  const std::string_view kStringValue = "test_string_value";
  const int kGroupValue = 1;
  const std::vector<std::string_view> kGroups = {"group_0", "group_1"};

  std::string buf_;
};

TEST_F(PatternPartFixture, StringType) {
  // Arrange
  const PatternPart pp(kStringValue);

  // Act
  pp.write(buf_, kGroups);

  // Assert
  EXPECT_EQ(buf_, kStringValue);
}

TEST_F(PatternPartFixture, GroupType) {
  // Arrange
  const PatternPart pp(kGroupValue);

  // Act
  pp.write(buf_, kGroups);

  // Assert
  EXPECT_EQ(buf_, kGroups[kGroupValue]);
}

struct FullMatchCase {
  std::string_view input;
  bool expected;
};

class RegexpFullMatchFixture : public testing::TestWithParam<FullMatchCase> {};

TEST_P(RegexpFullMatchFixture, Test) {
  // Arrange
  const Regexp rgx("job");

  // Act
  const auto result = rgx.full_match(GetParam().input);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Match, RegexpFullMatchFixture, testing::Values(FullMatchCase{.input = "job", .expected = true}));

INSTANTIATE_TEST_SUITE_P(NoMatch,
                         RegexpFullMatchFixture,
                         testing::Values(FullMatchCase{.input = "jobs", .expected = false},
                                         FullMatchCase{.input = "jo", .expected = false},
                                         FullMatchCase{.input = "jos", .expected = false},
                                         FullMatchCase{.input = "jobs", .expected = false},
                                         FullMatchCase{.input = "ajobs", .expected = false}));

struct GroupsCase {
  std::string_view regexp;
  int number_of_groups;
  std::map<std::string, int> expected;
};

class RegexpGroupsFixture : public testing::TestWithParam<GroupsCase> {};

TEST_P(RegexpGroupsFixture, Test) {
  // Arrange
  const Regexp rgx(GetParam().regexp);

  // Act
  const auto number_of_groups = rgx.number_of_capturing_groups();
  const auto groups = rgx.groups();

  // Assert
  EXPECT_EQ(GetParam().number_of_groups, number_of_groups);
  EXPECT_EQ(GetParam().expected, groups);
}

INSTANTIATE_TEST_SUITE_P(
    Tests,
    RegexpGroupsFixture,
    testing::Values(GroupsCase{.regexp = "job", .number_of_groups = 0, .expected = {{"0", 0}}},
                    GroupsCase{.regexp = "(b.*)", .number_of_groups = 1, .expected = {{"0", 0}, {"1", 1}}},
                    GroupsCase{.regexp = "f(.*);(.*)r", .number_of_groups = 2, .expected = {{"0", 0}, {"1", 1}, {"2", 2}}},
                    GroupsCase{.regexp = "(?P<name>[a-z]+)", .number_of_groups = 1, .expected = {{"0", 0}, {"1", 1}, {"name", 1}}},
                    GroupsCase{.regexp = "([1-9]+)-(?P<name>[a-z]+)", .number_of_groups = 2, .expected = {{"0", 0}, {"1", 1}, {"2", 2}, {"name", 2}}}));

struct MatchToArgsCase {
  std::string_view regexp;
  std::string_view input;
  bool expected;
  std::vector<std::string_view> matches;
};

class RegexpMatchToArgsFixture : public testing::TestWithParam<MatchToArgsCase> {};

TEST_P(RegexpMatchToArgsFixture, Test) {
  // Arrange
  const Regexp rgx(GetParam().regexp);
  std::vector<std::string_view> matches;

  // Act
  const auto result = rgx.match_to_args(GetParam().input, matches);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
  EXPECT_EQ(GetParam().matches, matches);
}

INSTANTIATE_TEST_SUITE_P(
    Tests,
    RegexpMatchToArgsFixture,
    testing::Values(MatchToArgsCase{.regexp = "job", .input = "job", .expected = true, .matches = {"job"}},
                    MatchToArgsCase{.regexp = "(b.*)", .input = "bar", .expected = true, .matches = {"bar", "bar"}},
                    MatchToArgsCase{.regexp = "(b.*)", .input = "boom", .expected = true, .matches = {"boom", "boom"}},
                    MatchToArgsCase{.regexp = "f(.*);(.*)r", .input = "foo;bar", .expected = true, .matches = {"foo;bar", "oo", "ba"}},
                    MatchToArgsCase{.regexp = "(?P<name>[a-z]+)", .input = "bvc", .expected = true, .matches = {"bvc", "bvc"}},
                    MatchToArgsCase{.regexp = "([1-9]+)-(?P<name>[a-z]+)", .input = "99-bvc", .expected = true, .matches = {"99-bvc", "99", "bvc"}},
                    MatchToArgsCase{.regexp = "([1-9]+)-(?P<name>[a-z]+)", .input = "aaa-bvc", .expected = false, .matches = {}}));

struct RelabelConfigCase {
  std::string_view replacement;
  std::string_view expected;
};

class RelabelConfigFixture : public testing::TestWithParam<RelabelConfigCase> {
 protected:
  GoRelabelConfig go_config_{
      .source_labels = {{"job"}},
      .separator = ";",
      .regex = "some-([^-]+)-(?P<name>[^,]+)",
      .modulus = 1000,
      .target_label = "$1${1}",
      .replacement = "$2${2}$$2${name}$name+",
      .action = rDrop,
  };

  static std::string parts_to_string(const std::vector<PatternPart>& parts) {
    static const std::vector<std::string_view> kGroups{"group_0", "group_1", "group_2"};

    std::string buf;

    for (auto& part : parts) {
      part.write(buf, kGroups);
    }

    return buf;
  }
};

TEST_F(RelabelConfigFixture, TestInit) {
  // Arrange

  // Act
  const RelabelConfig config{&go_config_};

  // Assert
  EXPECT_EQ(config.source_labels(), go_config_.source_labels);
  EXPECT_EQ(config.separator(), go_config_.separator);
  EXPECT_EQ(config.modulus(), go_config_.modulus);
  EXPECT_EQ(config.target_label(), go_config_.target_label);
  EXPECT_EQ(config.replacement(), go_config_.replacement);
  EXPECT_EQ(config.action(), go_config_.action);
}

TEST_F(RelabelConfigFixture, TargetLabel) {
  // Arrange
  const RelabelConfig config{&go_config_};

  // Act
  const auto parts = parts_to_string(config.target_label_parts());

  // Assert
  EXPECT_EQ("group_1group_1", parts);
}

TEST_P(RelabelConfigFixture, ReplacementTest) {
  // Arrange
  go_config_.replacement = GetParam().replacement;
  const RelabelConfig config{&go_config_};

  // Act
  const auto parts = parts_to_string(config.replacement_parts());

  // Assert
  EXPECT_EQ(GetParam().expected, parts);
}

INSTANTIATE_TEST_SUITE_P(Replacement,
                         RelabelConfigFixture,
                         testing::Values(RelabelConfigCase{.replacement = "$2${2}$$2${name}$name+", .expected = "group_2group_2$2group_2group_2+"}));
INSTANTIATE_TEST_SUITE_P(UnknownGroup,
                         RelabelConfigFixture,
                         testing::Values(RelabelConfigCase{.replacement = "${3}", .expected = ""}, RelabelConfigCase{.replacement = "$3", .expected = ""}));
INSTANTIATE_TEST_SUITE_P(UnknownGroupName,
                         RelabelConfigFixture,
                         testing::Values(RelabelConfigCase{.replacement = "${names}", .expected = ""},
                                         RelabelConfigCase{.replacement = "$names", .expected = ""}));
INSTANTIATE_TEST_SUITE_P(InvalidGroupName,
                         RelabelConfigFixture,
                         testing::Values(RelabelConfigCase{.replacement = "${name+}", .expected = "${name+}"},
                                         RelabelConfigCase{.replacement = "${name", .expected = "${name"},
                                         RelabelConfigCase{.replacement = "$", .expected = "$"}));

struct StatelessRelabelerCase {
  std::vector<GoRelabelConfig> configs;
  LabelViewSet labels;
  relabelStatus expected_status;
  LabelViewSet expected_labels;
};

class StatelessRelabelerFixture : public testing::TestWithParam<StatelessRelabelerCase> {
 protected:
  LabelsBuilder builder_;

  static std::vector<const GoRelabelConfig*> configs() {
    std::vector<const GoRelabelConfig*> result;
    result.reserve(GetParam().configs.size());
    for (auto& config : GetParam().configs) {
      result.emplace_back(&config);
    }
    return result;
  }
};

TEST_P(StatelessRelabelerFixture, Test) {
  // Arrange
  const StatelessRelabeler stateless_relabeler(configs());
  builder_.reset(GetParam().labels);

  // Act
  std::string buf;
  const auto status = stateless_relabeler.relabeling_process(buf, builder_);

  // Assert
  EXPECT_EQ(GetParam().expected_status, status);
  EXPECT_EQ(GetParam().expected_labels, builder_.label_view_set());
}

INSTANTIATE_TEST_SUITE_P(KeepEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"job"}, .regex = "abc", .action = rKeep}},
                                                                .labels = {{"__name__", "value"}, {"job", "abc"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "value"}, {"job", "abc"}}}));
INSTANTIATE_TEST_SUITE_P(KeepRegexpEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .regex = "b.*", .action = rKeep}},
                                                                .labels = {{"__name__", "boom"}, {"job", "abc"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "boom"}, {"job", "abc"}}}));
INSTANTIATE_TEST_SUITE_P(KeepNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"job"}, .regex = "no-match", .action = rKeep}},
                                                                .labels = {{"__name__", "value"}, {"job", "abs"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "value"}, {"job", "abs"}}}));
INSTANTIATE_TEST_SUITE_P(KeepNENoLabel,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"jub"}, .regex = "no-match", .action = rKeep}},
                                                                .labels = {{"__name__", "value"}, {"job", "abs"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "value"}, {"job", "abs"}}}));
INSTANTIATE_TEST_SUITE_P(KeepRegexpNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .regex = "b.*", .action = rKeep}},
                                                                .labels = {{"__name__", "zoom"}, {"job", "abc"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "zoom"}, {"job", "abc"}}}));
INSTANTIATE_TEST_SUITE_P(DropEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"job"}, .regex = "abc", .action = rDrop}},
                                                                .labels = {{"__name__", "value"}, {"job", "abc"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "value"}, {"job", "abc"}}}));
INSTANTIATE_TEST_SUITE_P(DropRegexpEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .regex = ".*o.*", .action = rDrop}},
                                                                .labels = {{"__name__", "boom"}, {"job", "beee"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "boom"}, {"job", "beee"}}}));
INSTANTIATE_TEST_SUITE_P(DropNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"job"}, .regex = "no-match", .action = rDrop}},
                                                                .labels = {{"__name__", "value"}, {"job", "abs"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "value"}, {"job", "abs"}}}));
INSTANTIATE_TEST_SUITE_P(DropRegexpNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .regex = "f|o", .action = rDrop}},
                                                                .labels = {{"__name__", "boom"}, {"job", "beee"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "boom"}, {"job", "beee"}}}));
INSTANTIATE_TEST_SUITE_P(DropRegexpNENoLabel,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"jub"}, .regex = "f|o", .action = rDrop}},
                                                                .labels = {{"__name__", "boom"}, {"job", "beee"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "boom"}, {"job", "beee"}}}));
INSTANTIATE_TEST_SUITE_P(DropEqualEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .target_label = "job", .action = rDropEqual}},
                                                                .labels = {{"__name__", "main"}, {"job", "main"}, {"instance", "else"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "main"}, {"job", "main"}, {"instance", "else"}}}));
INSTANTIATE_TEST_SUITE_P(DropEqualNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .target_label = "job", .action = rDropEqual}},
                                                                .labels = {{"__name__", "main"}, {"job", "ban"}, {"instance", "else"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "main"}, {"job", "ban"}, {"instance", "else"}}}));
INSTANTIATE_TEST_SUITE_P(KeepEqualEQ,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .target_label = "job", .action = rKeepEqual}},
                                                                .labels = {{"__name__", "main"}, {"job", "main"}, {"instance", "else"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "main"}, {"job", "main"}, {"instance", "else"}}}));
INSTANTIATE_TEST_SUITE_P(KeepEqualNE,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.source_labels = {"__name__"}, .target_label = "job", .action = rKeepEqual}},
                                                                .labels = {{"__name__", "main"}, {"job", "niam"}, {"instance", "else"}},
                                                                .expected_status = rsDrop,
                                                                .expected_labels = {{"__name__", "main"}, {"job", "niam"}, {"instance", "else"}}}));
INSTANTIATE_TEST_SUITE_P(Lowercase,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__"}, .target_label = "name_lowercase", .action = rLowercase}},
                             .labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}, {"name_lowercase", "lower_123_upper_123_case"}}}));
INSTANTIATE_TEST_SUITE_P(Uppercase,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__"}, .target_label = "name_uppercase", .action = rUppercase}},
                             .labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}, {"name_uppercase", "LOWER_123_UPPER_123_CASE"}}}));
INSTANTIATE_TEST_SUITE_P(LowercaseUppercase,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__"}, .target_label = "name_lowercase", .action = rLowercase},
                                         {.source_labels = {"__name__"}, .target_label = "name_uppercase", .action = rUppercase}},
                             .labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "lOwEr_123_UpPeR_123_cAsE"},
                                                 {"name_lowercase", "lower_123_upper_123_case"},
                                                 {"name_uppercase", "LOWER_123_UPPER_123_CASE"}}}));
INSTANTIATE_TEST_SUITE_P(HashMod,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"instance"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = rHashMod}},
                             .labels = {{"__name__", "eman"}, {"job", "boj"}, {"instance", "ecnatsni"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "eman"}, {"hash_mod", "72"}, {"job", "boj"}, {"instance", "ecnatsni"}}}));
INSTANTIATE_TEST_SUITE_P(HashMod2,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"instance"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = rHashMod}},
                             .labels = {{"__name__", "eman"}, {"job", "boj"}, {"instance", "ecna\ntsni"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "eman"}, {"hash_mod", "483"}, {"job", "boj"}, {"instance", "ecna\ntsni"}}}));
INSTANTIATE_TEST_SUITE_P(LabelMap,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.regex = "(j.*)", .replacement = "label_map_${1}", .action = rLabelMap}},
                             .labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}, {"label_map_jab", "baj"}, {"label_map_job", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(
    LabelMap2,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.regex = "meta_(ng.*)", .replacement = "${1}", .action = rLabelMap}},
        .labels = {{"__name__", "eman"}, {"meta_ng_jab", "baj"}, {"meta_ng_job", "boj"}, {"meta_jzb", "bzj"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "eman"}, {"meta_ng_jab", "baj"}, {"meta_ng_job", "boj"}, {"meta_jzb", "bzj"}, {"ng_jab", "baj"}, {"ng_job", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(LabelDrop,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.regex = "(j.*)", .replacement = "label_map_${1}", .action = rLabelDrop}},
                                                                .labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}},
                                                                .expected_status = rsRelabel,
                                                                .expected_labels = {{"__name__", "eman"}}}));
INSTANTIATE_TEST_SUITE_P(LabelDropTransparent,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.regex = "(j.*)", .replacement = "label_map_${1}", .action = rLabelDrop}},
                                                                .labels = {{"__name__", "eman"}, {"hab", "baj"}, {"hob", "boj"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"__name__", "eman"}, {"hab", "baj"}, {"hob", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(LabelDropFullDrop,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.regex = "(j.*)", .action = rLabelDrop}, {.regex = "(__.*)", .action = rLabelDrop}},
                             .labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {}}));
INSTANTIATE_TEST_SUITE_P(LabelDropFullDropAndAdd,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.regex = "(j.*)", .action = rLabelDrop},
                                         {.regex = "(__.*)", .action = rLabelDrop},
                                         {.source_labels = {"jab"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = rHashMod}},
                             .labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"hash_mod", "958"}}}));
INSTANTIATE_TEST_SUITE_P(LabelKeep,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.regex = "(j.*)", .replacement = "label_map_${1}", .action = rLabelKeep}},
                                                                .labels = {{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}},
                                                                .expected_status = rsRelabel,
                                                                .expected_labels = {{"jab", "baj"}, {"job", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(LabelKeepTransparent,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{.configs = {{.regex = "(j.*)", .replacement = "label_map_${1}", .action = rLabelKeep}},
                                                                .labels = {{"jab", "eman"}, {"job", "baj"}, {"jub", "boj"}},
                                                                .expected_status = rsKeep,
                                                                .expected_labels = {{"jab", "eman"}, {"job", "baj"}, {"jub", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(ReplaceToNewLS,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__"},
                                          .separator = ";",
                                          .regex = "e(.*)",
                                          .target_label = "replaced",
                                          .replacement = "ch${1}-ch${1}",
                                          .action = rReplace}},
                             .labels = {{"__name__", "eoo"}, {"jab", "baj"}, {"job", "boj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "eoo"}, {"jab", "baj"}, {"job", "boj"}, {"replaced", "choo-choo"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceToNewLS2,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .separator = ";", .regex = ".*(o).*", .target_label = "replaced", .replacement = "$1", .action = rReplace}},
        .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}}}));
INSTANTIATE_TEST_SUITE_P(ReplaceToNewLS3,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "tag", .action = rReplace}},
                             .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "tag"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceFullMatches,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"jub"}, .separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "tag", .action = rReplace}},
        .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "tag"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceNoMatches,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"jub"}, .separator = ";", .regex = "baj;(.*)g", .target_label = "replaced", .replacement = "tag", .action = rReplace}},
        .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}},
        .expected_status = rsKeep,
        .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceMatches,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs =
            {{.source_labels = {"jab", "job"}, .separator = ";", .regex = "baj;(.*)g", .target_label = "replaced", .replacement = "tag", .action = rReplace}},
        .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}, {"replaced", "tag"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceNoReplacement,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .separator = ";", .regex = "f", .target_label = "replaced", .replacement = "var", .action = rReplace}},
        .labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}},
        .expected_status = rsKeep,
        .expected_labels = {{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}}));
INSTANTIATE_TEST_SUITE_P(ReplaceBlankReplacement,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__"}, .regex = "(j).*", .target_label = "$1", .replacement = "$2", .action = rReplace}},
                             .labels = {{"__name__", "jazz"}, {"j", "baj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "jazz"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceCreateNewFromValue,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .regex = "some-([^-]+)-([^,]+)", .target_label = "${1}", .replacement = "${2}", .action = rReplace}},
        .labels = {{"__name__", "some-job2-boj"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "some-job2-boj"}, {"job2", "boj"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceInvalidLabelName,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .regex = "some-([^-]+)-([^,]+)", .target_label = "${1}", .replacement = "${2}", .action = rReplace}},
        .labels = {{"__name__", "some-2job-boj"}},
        .expected_status = rsKeep,
        .expected_labels = {{"__name__", "some-2job-boj"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceInvalidReplacement,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .regex = "some-([^-]+)-([^,]+)", .target_label = "${1}", .replacement = "${3}", .action = rReplace}},
        .labels = {{"__name__", "some-job-boj"}},
        .expected_status = rsKeep,
        .expected_labels = {{"__name__", "some-job-boj"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceInvalidTargetLabels,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__name__"}, .regex = "some-([^-]+)-([^,]+)", .target_label = "${3}", .replacement = "${1}", .action = rReplace},
                    {.source_labels = {"__name__"}, .regex = "some-([^-]+)-([^,]+)", .target_label = "${3}", .replacement = "${1}", .action = rReplace},
                    {.source_labels = {"__name__"}, .regex = "some-([^-]+)(-[^,]+)", .target_label = "${3}", .replacement = "${1}", .action = rReplace}},
        .labels = {{"__name__", "some-job-0"}},
        .expected_status = rsKeep,
        .expected_labels = {{"__name__", "some-job-0"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceComplexLikeUsecase,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"__meta_sd_tags"},
                     .regex = "(?:.+,|^)path:(/[^,]+).*",
                     .target_label = "__metrics_path__",
                     .replacement = "${1}",
                     .action = rReplace},
                    {.source_labels = {"__meta_sd_tags"}, .regex = "(?:.+,|^)job:([^,]+).*", .target_label = "job", .replacement = "${1}", .action = rReplace},
                    {.source_labels = {"__meta_sd_tags"},
                     .regex = "(?:.+,|^)label:([^=]+)=([^,]+).*",
                     .target_label = "${1}",
                     .replacement = "${2}",
                     .action = rReplace}},
        .labels = {{"__meta_sd_tags", "path:/secret,job:some-job,label:jab=baj"}},
        .expected_status = rsRelabel,
        .expected_labels =
            {{"__meta_sd_tags", "path:/secret,job:some-job,label:jab=baj"}, {"__metrics_path__", "/secret"}, {"job", "some-job"}, {"jab", "baj"}}}));
INSTANTIATE_TEST_SUITE_P(ReplaceIssues12283,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.regex = "^__meta_kubernetes_pod_container_port_name$", .action = rLabelDrop},
                                         {.source_labels = {"__meta_kubernetes_pod_annotation_XXX_metrics_port"},
                                          .regex = "(.+)",
                                          .target_label = "__meta_kubernetes_pod_container_port_name",
                                          .replacement = "metrics",
                                          .action = rReplace},
                                         {.source_labels = {"__meta_kubernetes_pod_container_port_name"}, .regex = "^metrics$", .action = rKeep}},
                             .labels = {{"__meta_kubernetes_pod_container_port_name", "foo"}, {"__meta_kubernetes_pod_annotation_XXX_metrics_port", "9091"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__meta_kubernetes_pod_annotation_XXX_metrics_port", "9091"},
                                                 {"__meta_kubernetes_pod_container_port_name", "metrics"}}}));
INSTANTIATE_TEST_SUITE_P(ReplaceWithReplace,
                         StatelessRelabelerFixture,
                         testing::Values(StatelessRelabelerCase{
                             .configs = {{.source_labels = {"__name__", "jab"},
                                          .separator = ";",
                                          .regex = "e(.*);(.*)j",
                                          .target_label = "__name__",
                                          .replacement = "b${1}${2}m",
                                          .action = rReplace},
                                         {.source_labels = {"job", "__name__"},
                                          .separator = ";",
                                          .regex = "(b).*b(.*)ba(.*)",
                                          .target_label = "replaced",
                                          .replacement = "$1$2$2$3",
                                          .action = rReplace}},
                             .labels = {{"__name__", "eoo"}, {"jab", "baj"}, {"job", "baj"}},
                             .expected_status = rsRelabel,
                             .expected_labels = {{"__name__", "boobam"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "boooom"}}}));
INSTANTIATE_TEST_SUITE_P(
    DropReplace,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs =
            {{.source_labels = {"__name__"}, .regex = ".*o.*", .action = rDrop},
             {.source_labels = {"__name__"}, .separator = ";", .regex = "e(.*)", .target_label = "replaced", .replacement = "ch$1-ch$1", .action = rReplace}},
        .labels = {{"__name__", "eoo"}, {"jab", "baj"}, {"job", "baj"}},
        .expected_status = rsDrop,
        .expected_labels = {{"__name__", "eoo"}, {"jab", "baj"}, {"job", "baj"}}}));
INSTANTIATE_TEST_SUITE_P(
    ReplaceWithReplaceJoin,
    StatelessRelabelerFixture,
    testing::Values(StatelessRelabelerCase{
        .configs = {{.source_labels = {"image", "name", "container"},
                     .separator = ";",
                     .regex = "(.+);(.+);",
                     .target_label = "container",
                     .replacement = "POD",
                     .action = rReplace}},
        .labels = {{"__name__", "fxample_metric"}, {"instance", "127.0.0.1:8080"}, {"image", "abr"}, {"name", "brbr"}},
        .expected_status = rsRelabel,
        .expected_labels = {{"__name__", "fxample_metric"}, {"container", "POD"}, {"instance", "127.0.0.1:8080"}, {"image", "abr"}, {"name", "brbr"}}}));

struct ProcessExternalLabelsCase {
  LabelViewSet labels;
  std::vector<LabelView> external_labels;
  LabelViewSet expected;
};

class ProcessExternalLabelsFixture : public testing::TestWithParam<ProcessExternalLabelsCase> {
 protected:
  LabelsBuilder builder_;
};

TEST_P(ProcessExternalLabelsFixture, Test) {
  // Arrange
  builder_.reset(GetParam().labels);

  // Act
  process_external_labels(builder_, GetParam().external_labels);

  // Assert
  EXPECT_EQ(GetParam().expected, builder_.label_view_set());
}

INSTANTIATE_TEST_SUITE_P(AddingLSEnd,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"a_name", "a_value"}},
                                                                   .external_labels = {{"c_name", "c_value"}},
                                                                   .expected = {{"a_name", "a_value"}, {"c_name", "c_value"}}}));
INSTANTIATE_TEST_SUITE_P(AddingLSBeginning,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"c_name", "c_value"}},
                                                                   .external_labels = {{"a_name", "a_value"}},
                                                                   .expected = {{"a_name", "a_value"}, {"c_name", "c_value"}}}));
INSTANTIATE_TEST_SUITE_P(OverrideExistingLabels,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"a_name", "a_value"}},
                                                                   .external_labels = {{"a_name", "b_value"}},
                                                                   .expected = {{"a_name", "a_value"}}}));
INSTANTIATE_TEST_SUITE_P(
    NoExternalLabels,
    ProcessExternalLabelsFixture,
    testing::Values(ProcessExternalLabelsCase{.labels = {{"a_name", "a_value"}}, .external_labels = {}, .expected = {{"a_name", "a_value"}}}));
INSTANTIATE_TEST_SUITE_P(
    NoLabels,
    ProcessExternalLabelsFixture,
    testing::Values(ProcessExternalLabelsCase{.labels = {}, .external_labels = {{"a_name", "a_value"}}, .expected = {{"a_name", "a_value"}}}));
INSTANTIATE_TEST_SUITE_P(LabelsLongerThanExternalLabels,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"a_name", "a_value"}, {"b_name", "b_value"}},
                                                                   .external_labels = {{"c_name", "c_value"}},
                                                                   .expected = {{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c_value"}}}));
INSTANTIATE_TEST_SUITE_P(ExternalLabelsLongerLabels,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"b_name", "b_value"}},
                                                                   .external_labels = {{"a_name", "a_value"}, {"c_name", "c_value"}},
                                                                   .expected = {{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c_value"}}}));
INSTANTIATE_TEST_SUITE_P(AddingWithWithoutClashingLabels,
                         ProcessExternalLabelsFixture,
                         testing::Values(ProcessExternalLabelsCase{.labels = {{"a_name", "a_value"}, {"b_name", "b_value"}},
                                                                   .external_labels = {{"a_name", "a1_value"}, {"b_name", "b1_value"}, {"c_name", "c1_value"}},
                                                                   .expected = {{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c1_value"}}}));

}  // namespace
