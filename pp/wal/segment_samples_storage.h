#pragma once

#include <chrono>

#include "bare_bones/sparse_vector.h"
#include "bare_bones/vector.h"
#include "primitives/sample.h"

namespace PromPP::WAL {

#pragma pack(push, 1)
class CompactSamplesList {
 public:
  using SamplesVector = BareBones::Vector<Primitives::Sample>;

  ~CompactSamplesList() {
    if (type_ == Type::kPlural) {
      sample_.plural.~SamplesVector();
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_single() const noexcept { return type_ == Type::kSingle; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_plural() const noexcept { return type_ == Type::kPlural; }

  void add(const Primitives::Sample& sample) {
    if (type_ == Type::kNone) [[likely]] {
      type_ = Type::kSingle;
      sample_.single = sample;
    } else if (type_ == Type::kSingle) {
      type_ = Type::kPlural;

      if (const auto sample_copy = sample_.single; sample.timestamp() >= sample_copy.timestamp()) [[likely]] {
        new (&sample_.plural) SamplesVector({sample_copy, sample});
      } else {
        new (&sample_.plural) SamplesVector({sample, sample_copy});
      }
    } else {
      if (auto& plural = sample_.plural; plural.back().timestamp() <= sample.timestamp()) [[likely]] {
        plural.emplace_back(sample);
      } else {
        const auto it = std::ranges::lower_bound(
            plural, sample, [](const Primitives::Sample& a, const Primitives::Sample& b) PROMPP_LAMBDA_INLINE { return a.timestamp() <= b.timestamp(); });
        plural.insert(it, sample);
      }
    }
  }

  [[nodiscard]] PROMPP_LAMBDA_INLINE const Primitives::Sample& sample() const noexcept {
    assert(type_ == Type::kSingle);
    return sample_.single;
  }

  [[nodiscard]] PROMPP_LAMBDA_INLINE const SamplesVector& samples() const noexcept {
    assert(type_ == Type::kPlural);
    return sample_.plural;
  }

 private:
  enum class Type : uint8_t {
    kNone = 0,
    kSingle,
    kPlural,
  };

  union SampleUnion {
    SampleUnion() {}
    ~SampleUnion() {}

    std::array<Primitives::Sample, 0> none{};
    Primitives::Sample single;
    SamplesVector plural;
  } sample_;
  Type type_{Type::kNone};
};
#pragma pack(pop)

}  // namespace PromPP::WAL

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::WAL::CompactSamplesList> : std::true_type {};

namespace PromPP::WAL {

class SegmentSamplesStorage {
 public:
  using SparseVector = BareBones::SparseVector<CompactSamplesList, BareBones::Vector>;
  using Iterator = SparseVector::Iterator;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t samples_count() const noexcept { return samples_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t series_count() const noexcept { return series_.items_count(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Timestamp earliest_sample() const noexcept { return earliest_sample_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Timestamp latest_sample() const noexcept { return latest_sample_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t first_sample_added_at_ts_ns() const noexcept { return first_sample_added_at_tsns_; }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    series_.clear();

    samples_count_ = 0;

    earliest_sample_ = std::numeric_limits<Primitives::Timestamp>::max();
    latest_sample_ = 0;
    first_sample_added_at_tsns_ = 0;
  }

  template <class T>
  PROMPP_ALWAYS_INLINE void add(Primitives::LabelSetID ls_id, const T& smpl) {
    if (first_sample_added_at_tsns_ == 0) [[unlikely]] {
      const auto now = std::chrono::system_clock::now();
      first_sample_added_at_tsns_ = std::chrono::duration_cast<std::chrono::nanoseconds>(now.time_since_epoch()).count();
    }

    earliest_sample_ = std::min(smpl.timestamp(), earliest_sample_);
    latest_sample_ = std::max(smpl.timestamp(), latest_sample_);
    ++samples_count_;

    if (ls_id >= series_.size()) [[unlikely]] {
      series_.resize(ls_id + 1 + kSeriesReserveSize);
    }

    series_[ls_id].add(smpl);
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  PROMPP_ALWAYS_INLINE void for_each(Callback&& func) const {
    for_each([&](Primitives::LabelSetID ls_id, const Primitives::Sample& sample) PROMPP_LAMBDA_INLINE { func(ls_id, sample.timestamp(), sample.value()); });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Sample>
  void for_each(Callback&& func) const {
    for (const auto& [ls_id, sample] : series_) {
      if (sample.is_single()) [[likely]] {
        func(ls_id, sample.sample());
      } else if (sample.is_plural()) [[likely]] {
        for (const auto& s : sample.samples()) {
          func(ls_id, s);
        }
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Iterator begin() const noexcept { return series_.begin(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static auto end() noexcept { return SparseVector::end(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return series_.empty(); }

 private:
  static constexpr auto kSeriesReserveSize = 512U;

  SparseVector series_;
  Primitives::Timestamp earliest_sample_{std::numeric_limits<Primitives::Timestamp>::max()};
  Primitives::Timestamp latest_sample_{};
  int64_t first_sample_added_at_tsns_{};
  uint32_t samples_count_{};
};

}  // namespace PromPP::WAL

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::WAL::SegmentSamplesStorage> : std::true_type {};
