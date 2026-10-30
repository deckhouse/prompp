#pragma once

#include <bit>
#include <cassert>
#include <concepts>
#include <cstddef>
#include <cstdint>
#include <iterator>
#include <limits>
#include <optional>
#include <ranges>
#include <span>
#include <type_traits>
#include <utility>

#include "arithmetic_progression.h"
#include "preprocess.h"
#include "stream_v_byte.h"
#include "zigzag.h"

namespace BareBones {
// length counts decoded values. Zero denotes an empty run.
struct EncodedRun {
  uint32_t code{};
  uint32_t length{};
};

namespace coding {
namespace detail {
// Persisted counts store length - 1. The current run remains buffered.
template <std::unsigned_integral Value>
  requires(sizeof(Value) <= sizeof(uint32_t))
class RunEncoder {
  using value_type = Value;

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
    if (count_ == std::numeric_limits<value_type>::max())
      last_ = val;

    if (val == last_) {
      ++count_;

      // The maximum count is reserved for the empty state.
      if (count_ == std::numeric_limits<value_type>::max() - 1) {
        *i++ = last_;
        *i++ = count_;
        clear();
      }
    } else {
      *i++ = last_;
      *i++ = count_;
      last_ = val;
      count_ = 0;
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

  [[nodiscard]] EncodedRun pending_run() const noexcept { return empty() ? EncodedRun{} : EncodedRun{last_, static_cast<uint32_t>(count_) + 1}; }
};
struct StoredRun {
  uint32_t code;
  uint32_t repeats;
};

// Requires a non-end source. A missing repeat count denotes a singleton.
template <class Iterator, class Sentinel>
StoredRun read_stored_run(Iterator& source, const Sentinel& end) noexcept {
  assert(source != end);
  const uint32_t code = *source;
  ++source;
  uint32_t repeats = 0;
  if (source != end) {
    repeats = static_cast<uint32_t>(*source);
    ++source;
  }
  return {code, repeats};
}

// Requires valid stored runs. The buffered tail follows the stored data.
// Returns an empty run after both sources are exhausted.
template <class Iterator, class Sentinel>
EncodedRun read_run(Iterator& source, const Sentinel& end, EncodedRun& tail) noexcept {
  if (source == end) {
    return std::exchange(tail, {});
  }
  const auto run = read_stored_run(source, end);
  assert(run.repeats != std::numeric_limits<uint32_t>::max());
  return {run.code, run.repeats + 1};
}
// Checks stored run lengths before applying coding-specific bounds.
template <class View, class Check>
bool is_valid_encoding(const View& store, Check check) noexcept {
  auto source = store.begin();
  const auto end = store.end();
  while (source != end) {
    const auto run = read_stored_run(source, end);
    if (run.repeats == std::numeric_limits<uint32_t>::max() || !check(EncodedRun{run.code, run.repeats + 1})) {
      return false;
    }
  }
  return true;
}
}  // namespace detail

// start_run() accepts a nonempty valid run after the previous run is fully emitted.
// next() returns nonempty progressions, then nullopt. Coding history persists between runs.
struct Identity {
  static uint32_t encode(uint32_t value) noexcept { return value; }

  template <class View>
  static bool is_valid_encoding(const View& store) noexcept {
    return detail::is_valid_encoding(store, [](EncodedRun) { return true; });
  }
  class Decoder {
    EncodedRun remaining_{};

   public:
    void start_run(EncodedRun run) noexcept {
      assert(remaining_.length == 0 && run.length != 0);
      remaining_ = run;
    }
    [[nodiscard]] std::optional<ArithmeticProgression> next() noexcept {
      if (remaining_.length == 0) {
        return std::nullopt;
      }
      const auto run = std::exchange(remaining_, {});
      return ArithmeticProgression{run.code, 0, run.length};
    }
  };
};

// Input values must be nondecreasing.
class Delta {
  uint32_t previous_{};

 public:
  uint32_t encode(uint32_t value) noexcept {
    assert(value >= previous_);
    return value - std::exchange(previous_, value);
  }

  template <class View>
  static bool is_valid_encoding(const View& store) noexcept {
    uint64_t previous = 0;
    return detail::is_valid_encoding(store, [&](EncodedRun run) {
      previous += static_cast<uint64_t>(run.code) * run.length;
      return previous <= std::numeric_limits<uint32_t>::max();
    });
  }
  class Decoder {
    uint32_t previous_{};
    EncodedRun remaining_{};

   public:
    void start_run(EncodedRun run) noexcept {
      assert(remaining_.length == 0 && run.length != 0);
      remaining_ = run;
    }
    [[nodiscard]] std::optional<ArithmeticProgression> next() noexcept {
      if (remaining_.length == 0) {
        return std::nullopt;
      }
      const auto run = std::exchange(remaining_, {});
      const uint32_t first = previous_ + run.code;
      if (run.length == 1) {
        previous_ = first;
        return ArithmeticProgression{first, 0, 1};
      }
      previous_ += run.code * run.length;
      return ArithmeticProgression{first, run.code, run.length};
    }
  };
};

class ZigZagDelta {
  uint32_t previous_{};

 public:
  uint32_t encode(uint32_t value) noexcept { return Encoding::ZigZag::encode(std::bit_cast<int32_t>(value - std::exchange(previous_, value))); }

  template <class View>
  static bool is_valid_encoding(const View& store) noexcept {
    return Identity::is_valid_encoding(store);
  }
  // Modular differences can wrap uint32_t. Each result stops before a wrap.
  class Decoder {
    uint32_t previous_{};
    EncodedRun remaining_{};

   public:
    void start_run(EncodedRun run) noexcept {
      assert(remaining_.length == 0 && run.length != 0);
      remaining_ = run;
    }
    [[nodiscard]] std::optional<ArithmeticProgression> next() noexcept {
      if (remaining_.length == 0) {
        return std::nullopt;
      }
      const int64_t step = Encoding::ZigZag::decode(remaining_.code);
      const uint32_t first = previous_ + step;
      const int64_t last = first + step * (remaining_.length - 1);
      uint32_t length = remaining_.length;
      if (last < 0) {
        length = first / -step + 1;
      } else if (last > std::numeric_limits<uint32_t>::max()) {
        length = (std::numeric_limits<uint32_t>::max() - first) / step + 1;
      }
      remaining_.length -= length;
      ArithmeticProgression result(first, step, length);
      previous_ = result.back();
      return result;
    }
  };
};
}  // namespace coding

// Access and iterator copies must not throw.
template <class View>
concept IntegerReadView =
    std::ranges::view<View> && std::default_initializable<View> && std::is_nothrow_copy_constructible_v<View> &&
    std::is_nothrow_copy_constructible_v<std::ranges::iterator_t<View>> && std::is_nothrow_copy_constructible_v<std::ranges::sentinel_t<View>> &&
    std::unsigned_integral<std::ranges::range_value_t<View>> && sizeof(std::ranges::range_value_t<View>) <= sizeof(uint32_t) &&
    requires(const View view, std::ranges::iterator_t<View> it, std::ranges::sentinel_t<View> end) {
      { view.begin() } -> std::same_as<std::ranges::iterator_t<View>>;
      { view.end() } -> std::same_as<std::ranges::sentinel_t<View>>;
      { static_cast<uint32_t>(*it) } noexcept;
      { ++it } noexcept -> std::same_as<std::ranges::iterator_t<View>&>;
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

// Borrows integer storage. Source mutation invalidates the view.
// Requires valid encoded runs, coding values, and a consistent tail.
// Check untrusted integer storage with Coding::is_valid_encoding() before constructing a view.
// Dereference and increment require a non-end iterator.
template <class Coding, IntegerReadView StoreView>
class RleView : public std::ranges::view_interface<RleView<Coding, StoreView>> {
  StoreView store_;
  EncodedRun tail_{};

  class RunCursor {
    std::ranges::iterator_t<StoreView> source_{};
    [[no_unique_address]] std::ranges::sentinel_t<StoreView> end_{};
    EncodedRun tail_{};
    [[no_unique_address]] typename Coding::Decoder decoder_{};

   public:
    RunCursor() = default;
    RunCursor(StoreView store, EncodedRun tail) noexcept : source_(store.begin()), end_(store.end()), tail_(tail) {}
    std::optional<ArithmeticProgression> next() noexcept {
      if (auto progression = decoder_.next()) {
        return progression;
      }
      const auto run = coding::detail::read_run(source_, end_, tail_);
      if (run.length == 0) {
        return std::nullopt;
      }
      decoder_.start_run(run);
      return decoder_.next();
    }
  };

 public:
  RleView() = default;
  RleView(StoreView store, EncodedRun tail = {}) noexcept : store_(store), tail_(tail) {}
  struct Sentinel {};
  class ProgressionIterator {
    RunCursor cursor_{};
    std::optional<ArithmeticProgression> current_{};

   public:
    using value_type = ArithmeticProgression;
    using difference_type = std::ptrdiff_t;
    using iterator_category = std::input_iterator_tag;
    using reference = const value_type&;
    using pointer = void;
    ProgressionIterator() = default;
    ProgressionIterator(StoreView store, EncodedRun tail) noexcept : cursor_(store, tail), current_(cursor_.next()) {}
    ProgressionIterator& operator++() noexcept {
      current_ = cursor_.next();
      return *this;
    }
    ProgressionIterator operator++(int) noexcept {
      auto copy = *this;
      ++*this;
      return copy;
    }
    [[nodiscard]] const ArithmeticProgression& operator*() const noexcept {
      assert(current_.has_value());
      return *current_;
    }
    [[nodiscard]] bool operator==(Sentinel) const noexcept { return !current_; }
  };
  class Iterator {
    RunCursor cursor_{};
    uint32_t value_{};
    uint32_t step_{};
    uint32_t remaining_{};

    void next_run() noexcept {
      if (const auto run = cursor_.next()) {
        value_ = run->first();
        step_ = static_cast<uint32_t>(run->step());
        remaining_ = run->size();
      } else {
        remaining_ = 0;
      }
    }

   public:
    using value_type = uint32_t;
    using difference_type = std::ptrdiff_t;
    using iterator_category = std::input_iterator_tag;
    using reference = value_type;
    using pointer = void;
    Iterator() = default;
    Iterator(StoreView store, EncodedRun tail) noexcept : cursor_(store, tail) { next_run(); }
    Iterator& operator++() noexcept {
      assert(remaining_ != 0);
      if (--remaining_ != 0) {
        value_ += step_;
      } else {
        next_run();
      }
      return *this;
    }
    Iterator operator++(int) noexcept {
      auto copy = *this;
      ++*this;
      return copy;
    }
    [[nodiscard]] uint32_t operator*() const noexcept {
      assert(remaining_ != 0);
      return value_;
    }
    [[nodiscard]] bool operator==(Sentinel) const noexcept { return remaining_ == 0; }
  };
  [[nodiscard]] Iterator begin() const noexcept { return {store_, tail_}; }
  [[nodiscard]] static Sentinel end() noexcept { return {}; }
  [[nodiscard]] auto progressions() const noexcept { return std::ranges::subrange(ProgressionIterator(store_, tail_), Sentinel{}); }
};

// Coding::encode preconditions apply to push_back(). Mutations preserve a valid encoding.
template <class Coding, RleStore Store = StreamVByte::Sequence<StreamVByte::Codec0124Frequent0>>
class RleSequence {
  Store store_{};
  coding::detail::RunEncoder<uint32_t> run_{};
  [[no_unique_address]] Coding encoder_{};

 public:
  using value_type = uint32_t;
  using View = RleView<Coding, typename Store::View>;
  using Iterator = typename View::Iterator;
  using IteratorSentinel = typename View::Sentinel;
  RleSequence() = default;
  RleSequence(const RleSequence&) = delete;
  RleSequence& operator=(const RleSequence&) = delete;
  RleSequence(RleSequence&& other) noexcept : store_(std::move(other.store_)), run_(std::move(other.run_)), encoder_(std::exchange(other.encoder_, {})) {}
  RleSequence& operator=(RleSequence&& other) noexcept {
    if (this != &other) {
      encoder_ = std::exchange(other.encoder_, {});
      run_ = std::move(other.run_);
      store_ = std::move(other.store_);
    }
    return *this;
  }
  void push_back(uint32_t value) noexcept {
    auto out = std::back_inserter(store_);
    run_.encode(encoder_.encode(value), out);
  }
  void flush() noexcept {
    auto out = std::back_inserter(store_);
    run_.flush(out);
  }
  void clear() noexcept {
    encoder_ = {};
    run_.clear();
    store_.clear();
  }
  [[nodiscard]] const Store& store() const noexcept { return store_; }
  [[nodiscard]] View view() const noexcept { return {store_.view(), run_.pending_run()}; }
  // Requires a complete record from a flushed sequence with compatible coding and storage.
  // Consumes that record. The caller must preserve its bytes and establish their integrity.
  [[nodiscard]] static View read_view(std::span<const uint8_t>& bytes) noexcept
    requires requires { Store::read_view(bytes); }
  {
    return View{Store::read_view(bytes)};
  }
  [[nodiscard]] auto progressions() const noexcept { return view().progressions(); }
  [[nodiscard]] Iterator begin() const noexcept { return view().begin(); }
  [[nodiscard]] static IteratorSentinel end() noexcept { return {}; }
  [[nodiscard]] size_t allocated_memory() const noexcept { return store_.allocated_memory(); }
  [[nodiscard]] bool is_flushed() const noexcept { return run_.empty(); }

  // Iterators borrow this snapshot. Synchronize capture and traversal with writes to the source.
  // Moving leaves the source empty.
  class Snapshot {
    [[no_unique_address]] typename Store::Snapshot store_{};
    EncodedRun tail_{};

   public:
    Snapshot() = default;
    Snapshot(const Snapshot&) = default;
    Snapshot& operator=(const Snapshot&) = default;
    Snapshot(Snapshot&& other) noexcept : store_(std::move(other.store_)), tail_(std::exchange(other.tail_, {})) {}
    Snapshot& operator=(Snapshot&& other) noexcept {
      if (this != &other) {
        store_ = std::move(other.store_);
        tail_ = std::exchange(other.tail_, {});
      }
      return *this;
    }
    Snapshot(typename Store::Snapshot store, EncodedRun tail) noexcept : store_(std::move(store)), tail_(tail) {}
    [[nodiscard]] auto view() const noexcept { return RleView<Coding, decltype(store_.view())>{store_.view(), tail_}; }
    [[nodiscard]] auto begin() const noexcept { return view().begin(); }
    [[nodiscard]] static IteratorSentinel end() noexcept { return {}; }
    [[nodiscard]] auto progressions() const noexcept { return view().progressions(); }
  };
  [[nodiscard]] Snapshot snapshot() const noexcept
    requires requires { store_.snapshot(); }
  {
    return {store_.snapshot(), run_.pending_run()};
  }
};

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
  // Requires a complete compatible record. The caller must establish its integrity.
  [[nodiscard]] static View read_view(std::span<const uint8_t>& bytes) noexcept { return View(Store::read_view(bytes)); }
  [[nodiscard]] Iterator begin() const noexcept { return view().begin(); }
  [[nodiscard]] static Sentinel end() noexcept { return {}; }
  [[nodiscard]] size_t allocated_memory() const noexcept { return store_.allocated_memory(); }

 private:
  Store store_{};
  uint32_t previous_{};
  uint32_t delta_{};
};

template <class Coding, RleStore Store>
struct IsTriviallyReallocatable<RleSequence<Coding, Store>>
    : std::bool_constant<std::is_trivially_copyable_v<Coding> && IsTriviallyReallocatable<Store>::value> {};
}  // namespace BareBones

namespace std::ranges {
template <>
inline constexpr bool enable_borrowed_range<BareBones::DeltaDeltaZigZagSequence::View> = true;
template <class Coding, BareBones::IntegerReadView StoreView>
inline constexpr bool enable_borrowed_range<BareBones::RleView<Coding, StoreView>> = enable_borrowed_range<StoreView>;
}  // namespace std::ranges
