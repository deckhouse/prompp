#pragma once

#include <bit>
#include <concepts>
#include <cstddef>
#include <cstdint>
#include <cstdlib>
#include <iterator>
#include <limits>
#include <ranges>
#include <span>
#include <type_traits>
#include <utility>

#include "arithmetic_progression.h"
#include "preprocess.h"
#include "stream_v_byte.h"
#include "zigzag.h"

namespace BareBones {
namespace encoding_detail {
// One owner retains the persisted (code, length - 1) format and the pending run.
template <class Store>
class RunEncoder {
  using value_type = typename Store::value_type;

  value_type count_{std::numeric_limits<value_type>::max()};
  value_type last_{std::numeric_limits<value_type>::max()};

 public:
  PROMPP_ALWAYS_INLINE RunEncoder() noexcept = default;
  RunEncoder(const RunEncoder&) = default;
  RunEncoder& operator=(const RunEncoder&) = default;

  PROMPP_ALWAYS_INLINE RunEncoder(RunEncoder&& o) noexcept : count_(o.count_), last_(o.last_) { o.clear(); }

  PROMPP_ALWAYS_INLINE RunEncoder& operator=(RunEncoder&& o) noexcept {
    count_ = o.count_;
    last_ = o.last_;
    o.clear();
    return *this;
  }

  template <std::output_iterator<value_type> IteratorType>
  PROMPP_ALWAYS_INLINE void encode(value_type val, IteratorType& i) noexcept {
    // assume last_ = val on first call
    if (count_ == std::numeric_limits<value_type>::max())
      last_ = val;

    if (val == last_) {
      ++count_;

      // check for overflow
      if (count_ == std::numeric_limits<value_type>::max() - 1) {
        *i++ = last_;
        *i++ = count_;
        clear();
      }
    } else {
      *i++ = last_;
      *i++ = count_;
      last_ = val;
      count_ = 0;  // use 0 to encode 1 occurrence
    }
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept { count_ = std::numeric_limits<value_type>::max(); }
  PROMPP_ALWAYS_INLINE bool empty() const noexcept { return count_ == std::numeric_limits<value_type>::max(); }

  template <std::output_iterator<value_type> IteratorType>
  PROMPP_ALWAYS_INLINE void flush(IteratorType& i) noexcept {
    if (count_ != std::numeric_limits<value_type>::max()) {
      *i++ = last_;
      *i++ = count_;
      clear();
    }
  }

  PROMPP_ALWAYS_INLINE value_type count() const noexcept { return count_; }
  PROMPP_ALWAYS_INLINE value_type last() const noexcept { return last_; }
};
struct Run {
  uint32_t code{};
  uint32_t length{};
};
// This is the only parser of persisted RLE pairs. A missing final count denotes a singleton.
template <class Iterator, class Sentinel>
Run read_run(Iterator& source, const Sentinel& end, Run& tail, bool& valid) noexcept {
  if (source == end) {
    return std::exchange(tail, {});
  }
  const uint32_t code = *source++;
  const uint32_t repeats = source == end ? 0U : static_cast<uint32_t>(*source++);
  if (repeats == std::numeric_limits<uint32_t>::max()) {
    valid = false;
    return {};
  }
  return {code, repeats + 1};
}
}  // namespace encoding_detail

namespace coding {
struct Identity {
  static uint32_t encode(uint32_t value) noexcept { return value; }
  static bool validate(encoding_detail::Run) noexcept { return true; }
  static bool has_pending() noexcept { return false; }
  static ArithmeticProgression decode(encoding_detail::Run run) noexcept { return {run.code, 0, run.length}; }
};
// Input values must be nondecreasing. Repeated values are permitted.
class Delta {
  uint32_t previous_{};

 public:
  uint32_t encode(uint32_t value) noexcept {
    assert(value >= previous_);
    return value - std::exchange(previous_, value);
  }
  // Validation advances its own cursor without constructing a progression.
  bool validate(encoding_detail::Run run) noexcept {
    const uint64_t last = uint64_t(previous_) + uint64_t(run.code) * run.length;
    if (last > std::numeric_limits<uint32_t>::max()) {
      return false;
    }
    previous_ = last;
    return true;
  }
  static bool has_pending() noexcept { return false; }
  ArithmeticProgression decode(encoding_detail::Run run) noexcept {
    const uint32_t first = previous_ + run.code;
    ArithmeticProgression result(first, run.code, run.length);
    previous_ = result.back();
    return result;
  }
};
// Differences use modulo-2^32 arithmetic. A persisted run can cross a uint32_t boundary.
// Exposed progressions stop at each boundary. The remaining run stays with this coding cursor.
class ZigZagDelta {
  uint32_t previous_{};
  encoding_detail::Run pending_{};

 public:
  uint32_t encode(uint32_t value) noexcept { return Encoding::ZigZag::encode(std::bit_cast<int32_t>(value - std::exchange(previous_, value))); }
  bool validate(encoding_detail::Run run) noexcept {
    previous_ += static_cast<uint32_t>(Encoding::ZigZag::decode(run.code)) * run.length;
    return true;
  }
  bool has_pending() const noexcept { return pending_.length != 0; }
  ArithmeticProgression decode(encoding_detail::Run run) noexcept {
    if (has_pending()) {
      assert(run.length == 0);
      run = pending_;
    }
    assert(run.length != 0);
    const int64_t step = Encoding::ZigZag::decode(run.code);
    const uint32_t first = previous_ + step;
    const int64_t last = first + step * (run.length - 1);
    uint32_t length = run.length;
    if (last < 0) {
      length = first / -step + 1;
    } else if (last > std::numeric_limits<uint32_t>::max()) {
      length = (std::numeric_limits<uint32_t>::max() - first) / step + 1;
    }
    pending_ = {run.code, run.length - length};
    ArithmeticProgression result(first, step, length);
    previous_ = result.back();
    return result;
  }
};
}  // namespace coding

// Use separate cursors for encoding, validation and decoding. validate advances over a complete run.
// decode emits one non-wrapping prefix. Drain has_pending() with decode({}) before supplying the next run.
template <class Coding>
concept RunCoding = std::is_nothrow_default_constructible_v<Coding> && std::is_nothrow_copy_constructible_v<Coding> &&
                    std::is_nothrow_copy_assignable_v<Coding> && std::is_nothrow_move_constructible_v<Coding> && std::is_nothrow_move_assignable_v<Coding> &&
                    requires(Coding coding, uint32_t value, encoding_detail::Run run) {
                      { coding.encode(value) } noexcept -> std::same_as<uint32_t>;
                      { coding.decode(run) } noexcept -> std::same_as<ArithmeticProgression>;
                      { coding.validate(run) } noexcept -> std::same_as<bool>;
                      { coding.has_pending() } noexcept -> std::same_as<bool>;
                    };

// View access must not throw. std::ranges::subrange copies its nonthrowing iterator and sentinel.
template <class View>
concept IntegerReadView =
    std::ranges::view<View> && std::default_initializable<View> && std::is_nothrow_copy_constructible_v<View> &&
    std::is_nothrow_copy_constructible_v<std::ranges::iterator_t<View>> && std::is_nothrow_copy_constructible_v<std::ranges::sentinel_t<View>> &&
    std::unsigned_integral<std::ranges::range_value_t<View>> && sizeof(std::ranges::range_value_t<View>) <= sizeof(uint32_t) &&
    requires(const View view, std::ranges::iterator_t<View> it, std::ranges::sentinel_t<View> end) {
      { view.begin() } -> std::same_as<std::ranges::iterator_t<View>>;
      { view.end() } -> std::same_as<std::ranges::sentinel_t<View>>;
      { static_cast<uint32_t>(*it) } noexcept;
      { it++ } noexcept;
      { it == end } noexcept;
    };

template <class Store>
concept RleStore =
    IntegerReadView<typename Store::View> && std::same_as<typename Store::value_type, uint32_t> && std::is_nothrow_move_constructible_v<Store> &&
    std::is_nothrow_move_assignable_v<Store> && std::is_nothrow_copy_constructible_v<typename Store::View> && requires(Store store, uint32_t code) {
      { store.push_back(code) } noexcept;
      { store.clear() } noexcept;
      { store.view() } noexcept;
      { store.allocated_memory() } noexcept;
    };

// A view borrows its integer storage. Mutation invalidates it. Its iterators copy the open run and coding state.
// Iteration requires valid pairs and coding-domain values. Check is_valid() before reading external integer stores.
template <RunCoding Coding, IntegerReadView StoreView>
class RleView : public std::ranges::view_interface<RleView<Coding, StoreView>> {
  StoreView store_;
  encoding_detail::Run tail_{};

 public:
  RleView() = default;
  RleView(StoreView store, encoding_detail::Run tail = {}) noexcept : store_(store), tail_(tail) {}
  struct Sentinel {};
  class Iterator;
  class ProgressionIterator {
    std::ranges::iterator_t<StoreView> source_{};
    [[no_unique_address]] std::ranges::sentinel_t<StoreView> end_{};
    encoding_detail::Run tail_{};
    [[no_unique_address]] Coding coding_{};
    ArithmeticProgression current_{};
    friend class Iterator;

   public:
    using value_type = ArithmeticProgression;
    using difference_type = std::ptrdiff_t;
    using iterator_category = std::input_iterator_tag;
    using reference = const value_type&;
    using pointer = void;
    ProgressionIterator() = default;
    ProgressionIterator(StoreView store, encoding_detail::Run tail) noexcept : source_(store.begin()), end_(store.end()), tail_(tail) { ++*this; }
    ProgressionIterator& operator++() noexcept {
      bool valid = true;
      const auto run = coding_.has_pending() ? encoding_detail::Run{} : encoding_detail::read_run(source_, end_, tail_, valid);
      // Unchecked iteration requires a valid store. External record readers validate before iteration.
      if (!valid) {
        std::abort();
      }
      current_ = run.length == 0 && !coding_.has_pending() ? ArithmeticProgression{} : coding_.decode(run);
      return *this;
    }
    ProgressionIterator operator++(int) noexcept {
      auto copy = *this;
      ++*this;
      return copy;
    }
    [[nodiscard]] const ArithmeticProgression& operator*() const noexcept { return current_; }
    [[nodiscard]] bool operator==(Sentinel) const noexcept { return current_.empty(); }
  };
  class Iterator {
    ProgressionIterator run_{};

   public:
    using value_type = uint32_t;
    using difference_type = std::ptrdiff_t;
    using iterator_category = std::input_iterator_tag;
    using reference = value_type;
    using pointer = void;
    Iterator() = default;
    Iterator(StoreView store, encoding_detail::Run tail) noexcept : run_(store, tail) {}
    Iterator& operator++() noexcept {
      auto& current = run_.current_;
      if (current.size() == 1) {
        ++run_;
      } else {
        current = ArithmeticProgression(current[1], current.step(), current.size() - 1);
      }
      return *this;
    }
    Iterator operator++(int) noexcept {
      auto copy = *this;
      ++*this;
      return copy;
    }
    [[nodiscard]] uint32_t operator*() const noexcept { return (*run_).first(); }
    [[nodiscard]] bool operator==(Sentinel end) const noexcept { return run_ == end; }
  };
  [[nodiscard]] bool is_valid() const noexcept {
    auto source = store_.begin();
    const auto end = store_.end();
    auto tail = tail_;
    Coding coding;
    bool valid = true;
    for (auto run = encoding_detail::read_run(source, end, tail, valid); run.length != 0; run = encoding_detail::read_run(source, end, tail, valid)) {
      if (!coding.validate(run)) {
        return false;
      }
    }
    return valid;
  }
  [[nodiscard]] Iterator begin() const noexcept { return {store_, tail_}; }
  [[nodiscard]] static Sentinel end() noexcept { return {}; }
  [[nodiscard]] auto progressions() const noexcept { return std::ranges::subrange(ProgressionIterator(store_, tail_), Sentinel{}); }
};

template <RunCoding Coding, RleStore Store = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
class RleSequence {
  Store store_{};
  encoding_detail::RunEncoder<Store> run_{};
  [[no_unique_address]] Coding coding_{};

 public:
  using value_type = uint32_t;
  using View = RleView<Coding, typename Store::View>;
  using Iterator = typename View::Iterator;
  using IteratorSentinel = typename View::Sentinel;
  RleSequence() = default;
  RleSequence(const RleSequence&) = delete;
  RleSequence& operator=(const RleSequence&) = delete;
  RleSequence(RleSequence&& other) noexcept : store_(std::move(other.store_)), run_(std::move(other.run_)), coding_(std::exchange(other.coding_, {})) {}
  RleSequence& operator=(RleSequence&& other) noexcept {
    if (this != &other) {
      coding_ = std::exchange(other.coding_, {});
      run_ = std::move(other.run_);
      store_ = std::move(other.store_);
    }
    return *this;
  }
  void push_back(uint32_t value) noexcept {
    auto out = std::back_inserter(store_);
    run_.encode(coding_.encode(value), out);
  }
  void flush() noexcept {
    auto out = std::back_inserter(store_);
    run_.flush(out);
  }
  void clear() noexcept {
    coding_ = {};
    run_.clear();
    store_.clear();
  }
  [[nodiscard]] const Store& store() const noexcept { return store_; }
  [[nodiscard]] View view() const noexcept { return {store_.view(), open_run()}; }
  [[nodiscard]] static View read_view(std::span<const uint8_t>& bytes) noexcept
    requires requires { Store::read_view(bytes); }
  {
    const View result{Store::read_view(bytes)};
    return result.is_valid() ? result : View{};
  }
  [[nodiscard]] auto progressions() const noexcept { return view().progressions(); }
  [[nodiscard]] Iterator begin() const noexcept { return view().begin(); }
  [[nodiscard]] static IteratorSentinel end() noexcept { return {}; }
  [[nodiscard]] size_t allocated_memory() const noexcept { return store_.allocated_memory(); }
  [[nodiscard]] bool is_flushed() const noexcept { return run_.empty(); }

  // Capture under writer exclusion. Iteration borrows this snapshot, not the source sequence.
  // Concurrent append and iteration can still race on shared key bytes and wide value reads.
  class Snapshot {
    [[no_unique_address]] typename Store::Snapshot store_{};
    encoding_detail::Run tail_{};

   public:
    Snapshot() = default;
    Snapshot(typename Store::Snapshot store, encoding_detail::Run tail) noexcept : store_(std::move(store)), tail_(tail) {}
    [[nodiscard]] auto view() const noexcept { return RleView<Coding, decltype(store_.view())>{store_.view(), tail_}; }
    [[nodiscard]] auto begin() const noexcept { return view().begin(); }
    [[nodiscard]] static IteratorSentinel end() noexcept { return {}; }
    [[nodiscard]] auto progressions() const noexcept { return view().progressions(); }
  };
  [[nodiscard]] Snapshot snapshot() const noexcept
    requires requires { store_.snapshot(); }
  {
    return {store_.snapshot(), open_run()};
  }

 private:
  [[nodiscard]] encoding_detail::Run open_run() const noexcept {
    return run_.empty() ? encoding_detail::Run{} : encoding_detail::Run{run_.last(), run_.count() + 1};
  }
};

// Chunk lengths use second differences. They have a scalar view and no RLE progression contract.
class DeltaDeltaZigZagSequence {
 public:
  using Store = StreamVByte::CompactSequence<StreamVByte::Codec0124Frequent0>;
  using value_type = uint32_t;
  struct Sentinel {};
  class View : public std::ranges::view_interface<View> {
    Store::View store_{};

   public:
    class Iterator {
      Store::const_iterator source_{};
      uint32_t previous_{};
      uint32_t delta_{};

     public:
      using value_type = uint32_t;
      using difference_type = std::ptrdiff_t;
      using iterator_category = std::input_iterator_tag;
      using reference = value_type;
      using pointer = void;
      Iterator() = default;
      explicit Iterator(Store::const_iterator source) noexcept : source_(source) {}
      Iterator& operator++() noexcept {
        delta_ += Encoding::ZigZag::decode(*source_);
        previous_ += delta_;
        ++source_;
        return *this;
      }
      Iterator operator++(int) noexcept {
        auto copy = *this;
        ++*this;
        return copy;
      }
      uint32_t operator*() const noexcept { return previous_ + delta_ + Encoding::ZigZag::decode(*source_); }
      bool operator==(Sentinel) const noexcept { return source_ == Store::end(); }
    };
    View() = default;
    explicit View(Store::View store) noexcept : store_(store) {}
    Iterator begin() const noexcept { return Iterator(store_.begin()); }
    static Sentinel end() noexcept { return {}; }
  };
  using Iterator = View::Iterator;
  DeltaDeltaZigZagSequence() = default;
  DeltaDeltaZigZagSequence(const DeltaDeltaZigZagSequence&) = delete;
  DeltaDeltaZigZagSequence& operator=(const DeltaDeltaZigZagSequence&) = delete;
  DeltaDeltaZigZagSequence(DeltaDeltaZigZagSequence&& other) noexcept
      : store_(std::move(other.store_)), previous_(std::exchange(other.previous_, 0)), delta_(std::exchange(other.delta_, 0)) {}
  DeltaDeltaZigZagSequence& operator=(DeltaDeltaZigZagSequence&& other) noexcept {
    if (this != &other) {
      store_ = std::move(other.store_);
      previous_ = std::exchange(other.previous_, 0);
      delta_ = std::exchange(other.delta_, 0);
    }
    return *this;
  }
  void push_back(uint32_t value) noexcept {
    const uint32_t next_delta = value - previous_;
    store_.push_back(Encoding::ZigZag::encode(std::bit_cast<int32_t>(next_delta - delta_)));
    delta_ = next_delta;
    previous_ = value;
  }
  void clear() noexcept {
    store_.clear();
    previous_ = 0;
    delta_ = 0;
  }
  static void flush() noexcept {}
  [[nodiscard]] const Store& store() const noexcept { return store_; }
  [[nodiscard]] View view() const noexcept { return View(store_.view()); }
  [[nodiscard]] static View read_view(std::span<const uint8_t>& bytes) noexcept { return View(Store::read_view(bytes)); }
  [[nodiscard]] Iterator begin() const noexcept { return view().begin(); }
  [[nodiscard]] static Sentinel end() noexcept { return {}; }
  [[nodiscard]] size_t allocated_memory() const noexcept { return store_.allocated_memory(); }

 private:
  Store store_{};
  uint32_t previous_{};
  uint32_t delta_{};
};

template <RunCoding Coding, RleStore Store>
struct IsTriviallyReallocatable<RleSequence<Coding, Store>>
    : std::bool_constant<std::is_trivially_copyable_v<Coding> && IsTriviallyReallocatable<Store>::value> {};
}  // namespace BareBones

namespace std::ranges {
template <>
inline constexpr bool enable_borrowed_range<BareBones::DeltaDeltaZigZagSequence::View> = true;
template <BareBones::RunCoding Coding, BareBones::IntegerReadView StoreView>
inline constexpr bool enable_borrowed_range<BareBones::RleView<Coding, StoreView>> = enable_borrowed_range<StoreView>;
}  // namespace std::ranges
