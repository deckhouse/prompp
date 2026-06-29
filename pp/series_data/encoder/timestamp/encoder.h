#pragma once

#include "metrics/metric.h"
#include "state.h"

namespace series_data::encoder::timestamp {

class TimestampEncoder {
 public:
  template <class BitSequenceWithItemsCount>
  static void encode_first(StateEncoder& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) {
    encoder.encode(timestamp, stream.stream);
  }

  template <class BitSequenceWithItemsCount>
  static void encode(StateEncoder& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) {
    if (stream.inc_count() == 1) [[unlikely]] {
      encoder.encode_delta(timestamp, stream.stream);
    } else {
      encoder.encode_delta_of_delta(timestamp, stream.stream);
    }
  }
};

class TimestampDecoder {
 public:
  explicit constexpr TimestampDecoder(const BareBones::BitSequenceReader& reader) : reader_(reader) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t decode() noexcept {
    if (gorilla_state_ == GorillaState::kFirstPoint) [[unlikely]] {
      decoder_.decode(reader_);
      gorilla_state_ = GorillaState::kSecondPoint;
    } else if (gorilla_state_ == GorillaState::kSecondPoint) [[unlikely]] {
      decoder_.decode_delta(reader_);
      gorilla_state_ = GorillaState::kOtherPoint;
    } else {
      decoder_.decode_delta_of_delta(reader_);
    }

    return decoder_.timestamp();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool eof() const noexcept { return reader_.eof(); }

  [[nodiscard]] static int64_t decode_first(BareBones::BitSequenceReader reader) noexcept {
    StateDecoder decoder;
    decoder.decode(reader);
    return decoder.timestamp();
  }

  [[nodiscard]] static BareBones::Vector<int64_t> decode_all(const BareBones::BitSequenceReader& reader, uint8_t count) noexcept {
    BareBones::Vector<int64_t> values;

    TimestampDecoder decoder(reader);
    for (uint8_t i = 0; i < count; ++i) {
      values.emplace_back(decoder.decode());
    }

    return values;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return decoder_.timestamp(); }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

  BareBones::BitSequenceReader reader_;
  StateDecoder decoder_;
  GorillaState gorilla_state_{GorillaState::kFirstPoint};
};

template <BareBones::ReallocatorInterface Reallocator>
class Encoder {
 private:
  using StateTransitions = timestamp::StateTransitions<Reallocator>;
  using State = timestamp::State<Reallocator>;
  using BitSequenceWithItemsCount = encoder::BitSequenceWithItemsCount<Reallocator>;

 public:
  StateId encode(StateId state_id, int64_t timestamp) {
    const auto hash = StateTransitions::hash(timestamp, state_id);

    if (const auto transition = state_transitions_.get(hash, timestamp, state_id); transition != nullptr) {
      const auto new_state_id = transition->state_id;
      if (state_id != kInvalidStateId) {
        decrease_reference_count(states_[state_id], state_id);
      }

      ++states_[new_state_id].reference_count;
      return new_state_id;
    }

    const auto previous_state_id = state_id;
    if (state_id == kInvalidStateId) [[unlikely]] {
      auto& state = emplace_state(state_id);
      TimestampEncoder::encode_first(state.encoder, timestamp, state.stream_data.stream);
      state_id = states_.index_of(state);
    } else {
      auto& new_state = emplace_state(state_id);

      auto& state = states_[state_id];
      ++state.child_count;

      if (state.reference_count > 1) [[likely]] {
        new_state = state;
      } else {
        new_state = std::move(state);
      }

      decrease_reference_count(state, state_id);
      state_id = states_.index_of(new_state);

      TimestampEncoder::encode(new_state.encoder, timestamp, new_state.stream_data.stream);
    }

    state_transitions_.emplace(hash, previous_state_id, state_id);
    return state_id;
  }

  PROMPP_ALWAYS_INLINE void erase(StateId state_id) { decrease_reference_count(states_[state_id], state_id); }

  PROMPP_ALWAYS_INLINE void finalize_or_copy(StateId state_id, BitSequenceWithItemsCount& stream, uint32_t finalized_stream_id) {
    if (auto& state = states_[state_id]; --state.reference_count == 0) {
      stream = state.finalize(finalized_stream_id);

      state_transitions_.erase(state);
      decrease_previous_state_child_count(state_id, state.previous_state_id);
      if (state.child_count == 0) {
        states_.erase(state_id);
      }
    } else {
      stream = state.stream_data.stream;
      stream.stream.shrink_to_fit();
    }
  }

  PROMPP_ALWAYS_INLINE void finalize(StateId state_id, BitSequenceWithItemsCount& stream, uint32_t finalized_stream_id) {
    auto& state = states_[state_id];
    stream = state.finalize(finalized_stream_id);
    decrease_reference_count(state, state_id);
  }

  PROMPP_ALWAYS_INLINE uint32_t process_finalized(StateId state_id) {
    if (auto& state = states_[state_id]; state.is_finalized()) [[unlikely]] {
      const auto result = state.stream_data.finalized_stream_id;
      decrease_reference_count(state, state_id);
      return result;
    }

    return kInvalidStateId;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return states_.allocated_memory() + state_transitions_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const BitSequenceWithItemsCount& get_stream(StateId state_id) const noexcept {
    return states_[state_id].stream_data.stream;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE State& get_state(StateId state_id) noexcept { return states_[state_id]; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const State& get_state(StateId state_id) const noexcept { return states_[state_id]; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const BareBones::VectorWithHoles<State>& get_states() const noexcept { return states_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_unique_state(StateId state_id) const noexcept {
    auto& state = states_[state_id];
    return state.reference_count == 1 && state.child_count == 0;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t states_count() const noexcept { return states_.size(); }

  // Binds the gauge that mirrors states_count(). The gauge is owned by the metrics page (which outlives this encoder), so
  // the count is pushed eagerly on state creation instead of being pulled from this encoder at scrape time. That removes a
  // use-after-free where a scrape could read states_count() through a dangling pointer after the owning DataStorage (and
  // this encoder) had been destroyed.
  PROMPP_ALWAYS_INLINE void set_states_count_gauge(metrics::Gauge* states_count_gauge) noexcept {
    states_count_gauge_ = states_count_gauge;
    if (states_count_gauge_ != nullptr) [[likely]] {
      states_count_gauge_->set(states_.size());
    }
  }

 private:
  BareBones::VectorWithHoles<State, Reallocator> states_;
  StateTransitions state_transitions_{states_};
  metrics::Gauge* states_count_gauge_{};

  // states_.size() (== states_count()) counts allocated slots and only ever grows here, when emplace_back appends a new
  // slot; erase just marks a hole and emplace_back reuses holes, so neither changes states_.size(). Therefore the gauge
  // only needs to be refreshed on state creation (not on erase). Pushing the exact size() keeps the gauge correct even
  // when a hole is reused, and doing it here (on the writer thread) means the scrape never touches the encoder.
  PROMPP_ALWAYS_INLINE State& emplace_state(StateId previous_state_id) {
    auto& state = states_.emplace_back(previous_state_id);
    if (states_count_gauge_ != nullptr) [[likely]] {
      states_count_gauge_->set(states_.size());
    }
    return state;
  }

  PROMPP_ALWAYS_INLINE void decrease_reference_count(State& state, StateId state_id) noexcept {
    if (--state.reference_count == 0) {
      state_transitions_.erase(state);
      decrease_previous_state_child_count(state_id, state.previous_state_id);
      if (state.child_count == 0) {
        states_.erase(state_id);
      } else {
        state.free_memory();
      }
    }
  }

  PROMPP_ALWAYS_INLINE void decrease_previous_state_child_count(uint32_t state_id, uint32_t previous_state_id) noexcept {
    while (previous_state_id != kInvalidStateId) {
      states_[state_id].previous_state_id = kInvalidStateId;
      auto& previous_state = states_[previous_state_id];

      assert(previous_state.child_count > 0);

      if (--previous_state.child_count == 0 && previous_state.reference_count == 0) {
        state_id = previous_state_id;
        previous_state_id = previous_state.previous_state_id;

        states_.erase(state_id);
        continue;
      }

      return;
    }
  }
};

}  // namespace series_data::encoder::timestamp