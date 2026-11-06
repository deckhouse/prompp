#include "bare_bones/streams.h"

#include <gtest/gtest.h>

#include <sstream>

namespace {

constexpr auto kReadExceptions = std::ios_base::failbit | std::ios_base::badbit | std::ios_base::eofbit;

class ScopedStreamExceptionsFixture : public testing::Test {
 protected:
  std::istringstream stream_{"x"};

  struct BodyFailure {};

  struct ThrowingMaskStream {
    std::ios_base::iostate mask = std::ios_base::failbit;

    [[nodiscard]] std::ios_base::iostate exceptions() const noexcept { return mask; }

    void exceptions(std::ios_base::iostate next_mask) {
      mask = next_mask;
      if (mask == std::ios_base::failbit) {
        throw BodyFailure{};
      }
    }
  };
};

TEST_F(ScopedStreamExceptionsFixture, InstallsMaskAndRestoresItAfterSuccess) {
  // Arrange
  stream_.exceptions(std::ios_base::badbit);
  std::ios_base::iostate installed_mask{};

  // Act
  {
    const auto guard = BareBones::scoped_stream_exceptions(stream_, kReadExceptions);
    installed_mask = stream_.exceptions();
  }

  // Assert
  EXPECT_EQ(kReadExceptions, installed_mask);
  EXPECT_EQ(std::ios_base::badbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::goodbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, RestoresMaskAfterEarlyReturn) {
  // Arrange
  stream_.exceptions(std::ios_base::badbit);
  const auto read = [&] {
    const auto guard = BareBones::scoped_stream_exceptions(stream_, kReadExceptions);
    return stream_.get();
  };

  // Act
  const auto value = read();

  // Assert
  EXPECT_EQ('x', value);
  EXPECT_EQ(std::ios_base::badbit, stream_.exceptions());
}

TEST_F(ScopedStreamExceptionsFixture, PreservesReadFailureAndErrorFlags) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);
  char buffer[2]{};
  const auto read = [&] {
    const auto guard = BareBones::scoped_stream_exceptions(stream_, kReadExceptions);
    stream_.read(buffer, sizeof(buffer));
  };

  // Act
  EXPECT_THROW(read(), std::ios_base::failure);

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::failbit | std::ios_base::eofbit, stream_.rdstate());
  EXPECT_EQ(1, stream_.gcount());
}

TEST_F(ScopedStreamExceptionsFixture, PreservesWriteFailureAndErrorFlags) {
  // Arrange
  std::stringbuf buffer{std::ios_base::in};
  std::ostream stream{&buffer};
  stream.exceptions(std::ios_base::badbit);
  const auto write = [&] {
    const auto guard = BareBones::scoped_stream_exceptions(stream, std::ios_base::failbit | std::ios_base::badbit);
    stream.put('x');
  };

  // Act
  EXPECT_THROW(write(), std::ios_base::failure);

  // Assert
  EXPECT_EQ(std::ios_base::badbit, stream.exceptions());
  EXPECT_EQ(std::ios_base::badbit, stream.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, RestoresMaskWhenInstallationThrows) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);
  stream_.setstate(std::ios_base::badbit);

  // Act
  EXPECT_THROW(static_cast<void>(BareBones::scoped_stream_exceptions(stream_, kReadExceptions)), std::ios_base::failure);

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::badbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, SuppressesRestorationFailureWhenInstallationAlsoThrows) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);
  ASSERT_THROW(stream_.setstate(std::ios_base::failbit), std::ios_base::failure);

  // Act
  EXPECT_THROW(static_cast<void>(BareBones::scoped_stream_exceptions(stream_, kReadExceptions)), std::ios_base::failure);

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::failbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, PreservesBodyExceptionWhenRestorationThrows) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);
  const auto run = [&] {
    const auto guard = BareBones::scoped_stream_exceptions(stream_, std::ios_base::goodbit);
    stream_.setstate(std::ios_base::failbit);
    throw BodyFailure{};
  };

  // Act
  EXPECT_THROW(run(), BodyFailure);

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::failbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, SuppressesRestorationFailureOnNormalExit) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);

  // Act
  {
    const auto guard = BareBones::scoped_stream_exceptions(stream_, std::ios_base::goodbit);
    stream_.setstate(std::ios_base::failbit);
  }

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::failbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, RestoresNestedMasksInReverseOrder) {
  // Arrange
  stream_.exceptions(std::ios_base::badbit);
  std::ios_base::iostate outer_mask{};

  // Act
  {
    const auto outer = BareBones::scoped_stream_exceptions(stream_, std::ios_base::failbit);
    {
      const auto inner = BareBones::scoped_stream_exceptions(stream_, kReadExceptions);
    }
    outer_mask = stream_.exceptions();
  }

  // Assert
  EXPECT_EQ(std::ios_base::failbit, outer_mask);
  EXPECT_EQ(std::ios_base::badbit, stream_.exceptions());
}

TEST_F(ScopedStreamExceptionsFixture, RestoresNestedMasksDuringUnwinding) {
  // Arrange
  stream_.exceptions(std::ios_base::failbit);
  char buffer[2]{};
  const auto read = [&] {
    const auto outer = BareBones::scoped_stream_exceptions(stream_, std::ios_base::failbit | std::ios_base::badbit);
    const auto inner = BareBones::scoped_stream_exceptions(stream_, kReadExceptions);
    stream_.read(buffer, sizeof(buffer));
  };

  // Act
  EXPECT_THROW(read(), std::ios_base::failure);

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream_.exceptions());
  EXPECT_EQ(std::ios_base::failbit | std::ios_base::eofbit, stream_.rdstate());
}

TEST_F(ScopedStreamExceptionsFixture, SuppressesNonIoRestorationException) {
  // Arrange
  ThrowingMaskStream stream;

  // Act
  {
    const auto guard = BareBones::scoped_stream_exceptions(stream, std::ios_base::badbit);
  }

  // Assert
  EXPECT_EQ(std::ios_base::failbit, stream.exceptions());
}

struct StreamExceptionMaskCase {
  const char* name;
  std::ios_base::iostate mask;
};

class ScopedStreamExceptionMaskFixture : public testing::TestWithParam<StreamExceptionMaskCase> {};

TEST_P(ScopedStreamExceptionMaskFixture, RestoresOriginalMaskAfterReadFailure) {
  // Arrange
  std::istringstream stream;
  stream.exceptions(GetParam().mask);
  const auto read = [&] {
    const auto guard = BareBones::scoped_stream_exceptions(stream, kReadExceptions);
    stream.get();
  };

  // Act
  EXPECT_THROW(read(), std::ios_base::failure);

  // Assert
  EXPECT_EQ(GetParam().mask, stream.exceptions());
  EXPECT_EQ(std::ios_base::failbit | std::ios_base::eofbit, stream.rdstate());
}

INSTANTIATE_TEST_SUITE_P(OriginalMasks,
                         ScopedStreamExceptionMaskFixture,
                         testing::Values(StreamExceptionMaskCase{.name = "Good", .mask = std::ios_base::goodbit},
                                         StreamExceptionMaskCase{.name = "Fail", .mask = std::ios_base::failbit},
                                         StreamExceptionMaskCase{.name = "Bad", .mask = std::ios_base::badbit},
                                         StreamExceptionMaskCase{.name = "Eof", .mask = std::ios_base::eofbit},
                                         StreamExceptionMaskCase{.name = "All", .mask = kReadExceptions}),
                         [](const auto& info) { return info.param.name; });

}  // namespace
