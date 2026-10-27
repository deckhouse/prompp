#include "profiling.h"
#include "shared_tests.h"

#include <gtest/gtest.h>

#include <memory>
#include <stdexcept>
#include <type_traits>

namespace {

using PromPP::Profiling::Marker;
using PromPP::Profiling::ScopedZone;

static_assert(!std::is_copy_constructible_v<Marker>);
static_assert(!std::is_move_constructible_v<Marker>);
static_assert(!std::is_copy_constructible_v<ScopedZone>);
static_assert(!std::is_move_constructible_v<ScopedZone>);

constexpr int kExpectedPayloadEvaluations = PROMPP_PROFILE(enabled) ? 1 : 0;

int function_site() {
  PROMPP_PROFILE(function);
  return 9;
}

int returning_site() {
  PROMPP_PROFILE(scope, "returning test site");
  return 7;
}

void throwing_site() {
  PROMPP_PROFILE(scope, "throwing test site");
  throw std::runtime_error("test exception");
}

int break_site() {
  int count = 0;
  for (int i = 0; i != 3; ++i) {
    PROMPP_PROFILE(scope, "break test site");
    ++count;
    break;
  }
  return count;
}

int continue_site() {
  int count = 0;
  for (int i = 0; i != 3; ++i) {
    PROMPP_PROFILE(scope, "continue test site");
    if (i == 0) {
      continue;
    }
    ++count;
  }
  return count;
}

int conditional_site(bool condition) {
  if (condition)
    PROMPP_PROFILE(scope, "conditional test site");
  else
    return 7;
  return 3;
}

template <class, class>
uint64_t payload(int& count) {
  ++count;
  return 5;
}

TEST(Profiling, FunctionScopePreservesResult) {
  // Arrange
  // Act
  const auto result = function_site();
  // Assert
  EXPECT_EQ(9, result);
}

TEST(Profiling, ScopePreservesReturn) {
  // Arrange
  // Act
  const auto result = returning_site();
  // Assert
  EXPECT_EQ(7, result);
}

TEST(Profiling, ScopePreservesException) {
  // Arrange
  // Act
  // Assert
  EXPECT_THROW(throwing_site(), std::runtime_error);
}

TEST(Profiling, ScopePreservesBreak) {
  // Arrange
  // Act
  const auto result = break_site();
  // Assert
  EXPECT_EQ(1, result);
}

TEST(Profiling, ScopePreservesContinue) {
  // Arrange
  // Act
  const auto result = continue_site();
  // Assert
  EXPECT_EQ(2, result);
}

TEST(Profiling, ScopePreservesElse) {
  // Arrange
  // Act
  const auto result = conditional_site(false);
  // Assert
  EXPECT_EQ(7, result);
}

TEST(Profiling, ScopeCanBeAnIfStatement) {
  // Arrange
  // Act
  const auto result = conditional_site(true);
  // Assert
  EXPECT_EQ(3, result);
}

TEST(Profiling, PayloadIsEvaluatedOnlyWhenEnabled) {
  // Arrange
  int evaluations = 0;
  PROMPP_PROFILE(named_scope, zone, "payload test site");
  // Act
  PROMPP_PROFILE(value, zone, ++evaluations);
  // Assert
  EXPECT_EQ(kExpectedPayloadEvaluations, evaluations);
}

TEST(Profiling, PayloadAcceptsTemplateCommas) {
  // Arrange
  int evaluations = 0;
  PROMPP_PROFILE(named_scope, zone, "template payload test site");
  // Act
  PROMPP_PROFILE(value, zone, payload<int, int>(evaluations));
  // Assert
  EXPECT_EQ(kExpectedPayloadEvaluations, evaluations);
}

TEST(Profiling, MarkerCreatesNonmovableZone) {
  // Arrange
  static const Marker marker{"manual test site"};
  // Act
  auto zone = marker.enter();
  zone.set_value(7);
  // Assert
  EXPECT_FALSE(std::is_move_constructible_v<decltype(zone)>);
}

TEST(Profiling, NestedScopesPreserveValues) {
  // Arrange
  int result = 0;
  PROMPP_PROFILE(scope, "outer test site");
  // Act
  {
    PROMPP_PROFILE(scope, "inner test site");
    result = 11;
  }
  // Assert
  EXPECT_EQ(11, result);
}

TEST(Profiling, ParentAcceptsValueAfterChildEnds) {
  // Arrange
  static const Marker marker{"restored parent test site"};
  auto parent = marker.enter();
  // Act
  {
    PROMPP_PROFILE(scope, "completed child test site");
  }
  // Assert
  EXPECT_NO_THROW(parent.set_value(11));
}

TEST(Profiling, ParentAcceptsValueAfterChildThrows) {
  // Arrange
  static const Marker marker{"unwound parent test site"};
  auto parent = marker.enter();
  // Act
  EXPECT_THROW(throwing_site(), std::runtime_error);
  // Assert
  EXPECT_NO_THROW(parent.set_value(13));
}

TEST(Profiling, InlineSiteCanBeSharedAcrossTranslationUnits) {
  // Arrange
  // Act
  PromPP::Profiling::Testing::shared_site();
  PromPP::Profiling::Testing::shared_site_from_another_translation_unit();
  // Assert
  SUCCEED();
}

TEST(Profiling, PublishesCallSiteForTraceValidation) {
  // Arrange
  const auto* expected_file = __FILE__;
  // Act
  constexpr int expected_line = __LINE__ + 1;
  PROMPP_PROFILE(scope, "callsite metadata test");
  // Assert
  RecordProperty("trace_source_file", expected_file);
  RecordProperty("trace_source_line", expected_line);
}

#if PROMPP_PROFILE(enabled) && !defined(NDEBUG)
TEST(ProfilingDeathTest, RejectsValueForParentWhileChildIsActive) {
  // Arrange
  static const Marker parent_marker{"parent value test site"};
  static const Marker child_marker{"child value test site"};
  auto parent = parent_marker.enter();
  auto child = child_marker.enter();
  // Act
  // Assert
  EXPECT_DEATH(parent.set_value(7), "innermost active zone");
}

TEST(ProfilingDeathTest, RejectsEndingParentBeforeChild) {
  // Arrange
  static const Marker parent_marker{"parent lifetime test site"};
  static const Marker child_marker{"child lifetime test site"};
  auto parent = std::make_unique<ScopedZone>(parent_marker);
  auto child = child_marker.enter();
  // Act
  // Assert
  EXPECT_DEATH(parent.reset(), "reverse order on their starting thread");
}
#endif

#if !PROMPP_PROFILE(enabled)
TEST(Profiling, DisabledPayloadDoesNotRequireZoneOrExpression) {
  // Arrange
  // Act
  PROMPP_PROFILE(value, missing_zone, missing_expression());
  // Assert
  SUCCEED();
}
#endif

}  // namespace
