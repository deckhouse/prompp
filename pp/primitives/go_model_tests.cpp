#include <gtest/gtest.h>

#include "primitives/go_model.h"
#include "primitives/label_set.h"

#include "primitives/snug_composites.h"

namespace {

using PromPP::Primitives::LabelView;
using PromPP::Primitives::LabelViewSet;
using PromPP::Primitives::Go::Label;
using PromPP::Primitives::Go::String;
using LabelSetBuilder = PromPP::Primitives::Go::LabelSetBuilder<LabelViewSet, std::vector, std::vector>;

using PromPP::Primitives::Go::operator""_gs;

struct IteratorCase {
  LabelViewSet label_set{};
  std::vector<Label> add{};
  std::vector<String> del{};
  std::vector<LabelView> expected{};
};

class LabelSetBuilderFixture : public ::testing::TestWithParam<IteratorCase> {};

TEST_P(LabelSetBuilderFixture, TestIterator) {
  // Arrange
  LabelSetBuilder builder(GetParam().label_set, GetParam().add, GetParam().del);
  std::vector<LabelView> actual;

  // Act
  std::ranges::copy(builder, std::back_inserter(actual));

  // Assert
  EXPECT_EQ(GetParam().expected, actual);
}

INSTANTIATE_TEST_SUITE_P(Empty, LabelSetBuilderFixture, testing::Values(IteratorCase{}));

INSTANTIATE_TEST_SUITE_P(Labels,
                         LabelSetBuilderFixture,
                         testing::Values(
                             IteratorCase{
                                 .label_set = {{"key", "value"}},
                                 .expected = {{"key", "value"}},
                             },
                             IteratorCase{
                                 .label_set = {{"key1", "value1"}, {"key2", "value2"}},
                                 .expected = {{"key1", "value1"}, {"key2", "value2"}},
                             }));

INSTANTIATE_TEST_SUITE_P(AddLabels,
                         LabelSetBuilderFixture,
                         testing::Values(
                             IteratorCase{
                                 .add = {Label{.name = "key"_gs, .value = "value"_gs}},
                                 .expected = {{"key", "value"}},
                             },
                             IteratorCase{
                                 .add = {Label{.name = "key1"_gs, .value = "value1"_gs}, Label{.name = "key2"_gs, .value = "value2"_gs}},
                                 .expected = {{"key1", "value1"}, {"key2", "value2"}},
                             }));

INSTANTIATE_TEST_SUITE_P(LabelsWithAddLabels,
                         LabelSetBuilderFixture,
                         testing::Values(
                             IteratorCase{
                                 .label_set = {{"a", "a"}},
                                 .add = {Label{.name = "a"_gs, .value = "a"_gs}},
                                 .expected = {{"a", "a"}},
                             },
                             IteratorCase{
                                 .label_set = {{"b", "b"}},
                                 .add = {Label{.name = "a"_gs, .value = "a"_gs}},
                                 .expected = {{"a", "a"}, {"b", "b"}},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"b", "b"}},
                                 .add = {Label{.name = "a"_gs, .value = "a"_gs}},
                                 .expected = {{"a", "a"}, {"b", "b"}},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"b", "b"}},
                                 .add = {Label{.name = "a"_gs, .value = "a"_gs}, Label{.name = "c"_gs, .value = "c"_gs}},
                                 .expected = {{"a", "a"}, {"b", "b"}, {"c", "c"}},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"c", "c"}},
                                 .add = {Label{.name = "b"_gs, .value = "b"_gs}, Label{.name = "d"_gs, .value = "d"_gs}},
                                 .expected = {{"a", "a"}, {"b", "b"}, {"c", "c"}, {"d", "d"}},
                             }));

INSTANTIATE_TEST_SUITE_P(DelLabels,
                         LabelSetBuilderFixture,
                         testing::Values(
                             IteratorCase{
                                 .label_set = {{"a", "a"}},
                                 .del = {{"a"_gs}},
                                 .expected = {},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"b", "b"}},
                                 .del = {{"b"_gs}},
                                 .expected = {{"a", "a"}},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"b", "b"}},
                                 .add = {Label{.name = "b"_gs, .value = "b"_gs}},
                                 .del = {{"b"_gs}},
                                 .expected = {{"a", "a"}},
                             },
                             IteratorCase{
                                 .label_set = {{"a", "a"}, {"b", "b"}},
                                 .add = {Label{.name = "b"_gs, .value = "b"_gs}},
                                 .del = {{"a"_gs}, {"b"_gs}},
                                 .expected = {},
                             },
                             IteratorCase{
                                 .label_set = {{"b", "b"}, {"c", "c"}},
                                 .add = {Label{.name = "d"_gs, .value = "d"_gs}},
                                 .del = {{"a"_gs}, {"e"_gs}},
                                 .expected = {{"b", "b"}, {"c", "c"}, {"d", "d"}},
                             }));

}  // namespace