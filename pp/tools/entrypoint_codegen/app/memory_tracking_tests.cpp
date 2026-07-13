#include "app/memory_tracking.h"

#include <gtest/gtest.h>

#include <cstddef>

namespace {

TEST(TrackingMemoryResourceTest, SnapshotReportsAllocatedDeallocatedAndPeakLiveBytes) {
  // Arrange
  epgen::app::TrackingMemoryResource memory_resource;

  // Act
  void* first = memory_resource.allocate(8, alignof(std::max_align_t));
  void* second = memory_resource.allocate(4, alignof(std::max_align_t));
  const epgen::app::MemoryUsageSnapshot after_allocations = memory_resource.snapshot();
  memory_resource.deallocate(first, 8, alignof(std::max_align_t));
  const epgen::app::MemoryUsageSnapshot after_first_deallocate = memory_resource.snapshot();
  memory_resource.deallocate(second, 4, alignof(std::max_align_t));
  const epgen::app::MemoryUsageSnapshot after_all_deallocated = memory_resource.snapshot();

  // Assert
  EXPECT_EQ(after_allocations.allocated_bytes, 12);
  EXPECT_EQ(after_allocations.deallocated_bytes, 0);
  EXPECT_EQ(after_allocations.peak_live_bytes, 12);
  EXPECT_EQ(after_first_deallocate.allocated_bytes, 12);
  EXPECT_EQ(after_first_deallocate.deallocated_bytes, 8);
  EXPECT_EQ(after_first_deallocate.peak_live_bytes, 12);
  EXPECT_EQ(after_all_deallocated.allocated_bytes, 12);
  EXPECT_EQ(after_all_deallocated.deallocated_bytes, 12);
  EXPECT_EQ(after_all_deallocated.peak_live_bytes, 12);
}

}  // namespace
