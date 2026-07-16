#include "bare_bones/exception.h"

#include <array>
#include <cstddef>
#include <exception>
#include <string>
#include <string_view>

#include <gtest/gtest.h>

namespace {

TEST(ExceptionTest, ExposesCode) {
  // Arrange
  constexpr BareBones::Exception::Code kCode = 0xcc63de60c4c06e86;

  // Act
  const BareBones::Exception exception(kCode, "message");

  // Assert
  EXPECT_EQ(kCode, exception.code());
}

TEST(ExceptionTest, FormatsMessage) {
  // Arrange
  constexpr std::string_view kExpectedMessage = "Exception cc63de60c4c06e86: series 42";

  // Act
  const BareBones::Exception exception(0xcc63de60c4c06e86, "series %d", 42);

  // Assert
  EXPECT_EQ(kExpectedMessage, exception.message());
  EXPECT_EQ(kExpectedMessage, std::string_view(exception.what()));
}

TEST(ExceptionTest, ClampsFormattedMessageToSupportedLength) {
  // Arrange
  constexpr std::size_t kMaximumFormattedMessageSize = 255;
  std::array<char, kMaximumFormattedMessageSize + 2> source_message;
  source_message.fill('x');
  source_message.back() = '\0';
  const std::string expected_message = "Exception cc63de60c4c06e86: " + std::string(kMaximumFormattedMessageSize, 'x');

  // Act
  const BareBones::Exception exception(0xcc63de60c4c06e86, "%s", source_message.data());

  // Assert
  EXPECT_EQ(expected_message, exception.message());
  EXPECT_EQ(expected_message, std::string_view(exception.what()));
}

class ExceptionThrowFixture : public testing::Test {
 protected:
  static constexpr std::string_view kExpectedMessage = "Exception cc63de60c4c06e86: series 42";

  [[noreturn]] static void throw_exception() { throw BareBones::Exception(0xcc63de60c4c06e86, "series %d", 42); }

  static std::string_view catch_exception_as_std_exception() {
    try {
      throw_exception();
    } catch (const std::exception& exception) {
      return exception.what();
    }

    return {};
  }
};

TEST_F(ExceptionThrowFixture, ThrowsAsBareBonesException) {
  // Arrange

  // Act

  // Assert
  EXPECT_THROW(throw_exception(), BareBones::Exception);
}

TEST_F(ExceptionThrowFixture, CanBeCaughtAsStdException) {
  // Arrange

  // Act
  const std::string_view caught_message = catch_exception_as_std_exception();

  // Assert
  EXPECT_EQ(kExpectedMessage, caught_message);
}

}  // namespace
