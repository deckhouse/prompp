#pragma once

#include <algorithm>
#include <vector>

#include "snappy-sinksource.h"
#include "snappy.h"
#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/pbf_writer.hpp"

#include "bare_bones/preprocess.h"
#include "bare_bones/serializer.h"
#include "primitives/go_slice.h"
#include "primitives/labels_builder.h"
#include "primitives/snug_composites.h"
#include "prometheus/remote_write.h"
#include "prometheus/stateless_relabeler.h"
#include "segment_samples_storage.h"
#include "wal.h"

namespace PromPP::WAL {

struct RefSample {
  uint32_t id;
  int64_t t;
  double v;

  PROMPP_ALWAYS_INLINE bool operator==(const RefSample&) const noexcept = default;
};

struct ShardRefSample {
  Primitives::Go::SliceView<RefSample> ref_samples;
  uint16_t shard_id{};
};

class OutputDecoderCache {
 public:
  static constexpr auto kIsDropped = Primitives::kInvalidLabelSetID;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return cache_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return cache_.size(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool have_changes() const noexcept { return cache_.size() > dumped_cache_size_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::LabelSetID operator[](Primitives::LabelSetID source_ls_id) const noexcept { return cache_[source_ls_id]; }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t size) noexcept { cache_.reserve(size); }
  PROMPP_ALWAYS_INLINE void add_dropped() noexcept { add(kIsDropped); }
  PROMPP_ALWAYS_INLINE void add(Primitives::LabelSetID ls_id) noexcept { cache_.emplace_back(ls_id); }

  PROMPP_ALWAYS_INLINE void dump_changes(std::ostream& out) {
    BareBones::serialize(out, std::span{&cache_[dumped_cache_size_], cache_.size() - dumped_cache_size_});
    dumped_cache_size_ = cache_.size();
  }

  PROMPP_ALWAYS_INLINE void load_changes(std::istream& in) {
    BareBones::deserialize(in, cache_);
    dumped_cache_size_ = cache_.size();
  }

  bool operator==(const OutputDecoderCache& other) const noexcept { return cache_ == other.cache_; }

 private:
  BareBones::Vector<uint32_t> cache_{};
  uint32_t dumped_cache_size_{};
};

class GorillaSampleDecoderWithSkips {
 public:
  Primitives::Timestamp timestamp_base{std::numeric_limits<Primitives::Timestamp>::max()};

  [[nodiscard]] PROMPP_ALWAYS_INLINE OutputDecoderCache& cache() noexcept { return cache_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const OutputDecoderCache& cache() const noexcept { return cache_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
    return cache_.allocated_memory() + gorilla_decoders_.allocated_memory() + null_gorilla_decoders_.allocated_memory();
  }

  PROMPP_ALWAYS_INLINE static void load(std::istream&) {}

  PROMPP_ALWAYS_INLINE static void set_series_count(Primitives::LabelSetID) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode(Primitives::LabelSetID ls_id,
                                                               Primitives::Timestamp timestamp,
                                                               BareBones::BitSequenceReader& value_sequence,
                                                               SampleCrc&) {
    return decode_impl(ls_id, timestamp, value_sequence);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode(Primitives::LabelSetID ls_id,
                                                               BareBones::BitSequenceReader& timestamp_sequence,
                                                               BareBones::BitSequenceReader& value_sequence,
                                                               SampleCrc&) {
    return decode_impl(ls_id, timestamp_sequence, value_sequence);
  }

  PROMPP_ALWAYS_INLINE static SampleCrc::ValidationResult validate_crc(SampleCrc, SampleCrc) noexcept { return SampleCrc::ValidationResult::kValid; }

  PROMPP_ALWAYS_INLINE void sync_decoders_with_cache() {
    const auto size_before = null_gorilla_decoders_.size();
    null_gorilla_decoders_.resize(cache_.size());

    auto ls_id = gorilla_decoders_.size();
    for (auto index = size_before; index < null_gorilla_decoders_.size(); ++index) {
      if (cache_[index] != OutputDecoderCache::kIsDropped) {
        null_gorilla_decoders_[index].id = ls_id++;
      }
    }

    gorilla_decoders_.resize(ls_id);
  }

 private:
  union PROMPP_ATTRIBUTE_PACKED NullGorillaDecoderOrId {
    NullGorillaDecoder decoder;
    uint32_t id{};
  };

  OutputDecoderCache cache_;
  BareBones::Vector<GorillaDecoder> gorilla_decoders_;
  BareBones::Vector<NullGorillaDecoderOrId> null_gorilla_decoders_;

  template <class Timestamp>
  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Sample decode_impl(Primitives::LabelSetID source_ls_id,
                                                                    Timestamp&& timestamp,
                                                                    BareBones::BitSequenceReader& value_sequence) {
    if (source_ls_id >= cache_.size()) [[unlikely]] {
      throw BareBones::Exception(0xf0e57d2a0e5ce7ed, "Error while processing segment LabelSets: Unknown segment's LabelSet's id %d", source_ls_id);
    }

    if (const auto id = cache_[source_ls_id]; id == OutputDecoderCache::kIsDropped) {
      null_gorilla_decoders_[source_ls_id].decoder.decode(timestamp, value_sequence);
      return {};
    }

    auto& gorilla = gorilla_decoders_[null_gorilla_decoders_[source_ls_id].id];
    gorilla.decode(timestamp, value_sequence);
    return {gorilla.last_timestamp() + timestamp_base, gorilla.last_value()};
  }
};

static_assert(SampleDecoderInterface<GorillaSampleDecoderWithSkips>);

using BaseOutputDecoder = BasicDecoder<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<BareBones::Vector>, GorillaSampleDecoderWithSkips>;

template <class EncodingBimap>
class OutputDecoder : private BaseOutputDecoder {
  Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<BareBones::Vector> wal_lss_;
  std::string buf_;
  Primitives::LabelsBuilder builder_;
  std::vector<Primitives::LabelView> external_labels_{};
  Prometheus::Relabel::StatelessRelabeler& stateless_relabeler_;
  EncodingBimap& output_lss_;
  typename EncodingBimap::checkpoint_type dumped_checkpoint_{output_lss_.checkpoint()};

  uint32_t add_series_count_{};
  uint32_t dropped_series_count_{};

  // align_cache_to_lss add new labels from lss via relabeler to cache.
  PROMPP_ALWAYS_INLINE void align_cache_to_lss() {
    add_series_count_ = 0;
    dropped_series_count_ = 0;
    auto& cache = sample_decoder().cache();
    if (wal_lss_.max_item_index() <= cache.size()) {
      return;
    }

    cache.reserve(wal_lss_.max_item_index());
    for (size_t ls_id = cache.size(); ls_id < wal_lss_.max_item_index(); ++ls_id) {
      builder_.reset(wal_lss_[ls_id]);
      Prometheus::Relabel::process_external_labels(builder_, external_labels_);
      Prometheus::Relabel::relabelStatus rstatus = stateless_relabeler_.relabeling_process(buf_, builder_);
      Prometheus::Relabel::soft_validate(rstatus, builder_);

      if (rstatus == Prometheus::Relabel::rsDrop) {
        cache.add_dropped();
        ++dropped_series_count_;
      } else {
        cache.add(output_lss_.find_or_emplace(builder_.label_view_set()));
        ++add_series_count_;
      }
    }

    wal_lss_.shrink_to_checkpoint_size(wal_lss_.checkpoint());
    sample_decoder().sync_decoders_with_cache();
  }

  // load_segment override private load_segment from BaseOutputDecoder.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_segment(InputStream& in, BaseOutputDecoder& wal) {
    in >> wal;
  }

 public:
  // WALOutputDecoder constructor with empty state.
  PROMPP_ALWAYS_INLINE explicit OutputDecoder(Prometheus::Relabel::StatelessRelabeler& stateless_relabeler,
                                              EncodingBimap& output_lss,
                                              Primitives::Go::SliceView<std::pair<Primitives::Go::String, Primitives::Go::String>>& external_labels,
                                              BasicEncoderVersion encoder_version = Writer::version)
      : BaseOutputDecoder{wal_lss_, encoder_version}, stateless_relabeler_{stateless_relabeler}, output_lss_(output_lss) {
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // cache return current cache.
  PROMPP_ALWAYS_INLINE const auto& cache() const noexcept { return sample_decoder().cache(); }

  // add_series_count return number of add series after load segment.
  PROMPP_ALWAYS_INLINE uint64_t add_series_count() const noexcept { return add_series_count_; }

  // dropped_series_count return number of dropped series after load segment.
  PROMPP_ALWAYS_INLINE uint64_t dropped_series_count() const noexcept { return dropped_series_count_; }

  // dump_to dump delta state(delta caches and delta checkpoint lss) to output stream.
  PROMPP_ALWAYS_INLINE void dump_to(std::ostream& out) {
    // take current checkpoint and delta with current and previous checkpoints
    auto current_cp = output_lss_.checkpoint();
    const auto delta_cp = current_cp - dumped_checkpoint_;

    // no changes - do nothing
    if (delta_cp.empty() && !cache().have_changes()) [[unlikely]] {
      return;
    }

    // write dump type lss and write delta checkpoints
    output_lss_.save(out, delta_cp);
    dumped_checkpoint_ = std::move(current_cp);

    // write dump type cache and write delta caches
    sample_decoder().cache().dump_changes(out);
  }

  // load_from load state(lss and cache) from incoming stream.
  template <class InputStream>
  PROMPP_ALWAYS_INLINE void load_from(InputStream& in) {
    while (true) {
      if (in.eof()) {
        dumped_checkpoint_ = output_lss_.checkpoint();
        sample_decoder().sync_decoders_with_cache();
        return;
      }

      output_lss_.load(in);
      sample_decoder().cache().load_changes(in);
    }
  }

  // operator>> override friend operator from BaseOutputDecoder.
  template <class InputStream>
  friend InputStream& operator>>(InputStream& in, OutputDecoder& wal) {
    wal.load_segment(in, wal);
    wal.align_cache_to_lss();
    return in;
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type, bool>
  void process_segment(Callback&& func) {
    BaseOutputDecoder::process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      auto id = sample_decoder().cache()[ls_id];
      func(id, ts, v, id == OutputDecoderCache::kIsDropped);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, Primitives::Timestamp, Primitives::Sample::value_type>
  void process_segment(Callback&& func) {
    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v, bool is_dropped) {
      if (is_dropped) {
        return;
      }

      func(ls_id, ts, v);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, label_set_type, Primitives::Timestamp, Primitives::Sample::value_type>
  void process_segment(Callback&& func) {
    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      const auto& label_set = output_lss_[ls_id];

      func(label_set, ts, v);
    });
  }

  template <class Callback>
    requires std::is_invocable_v<Callback, Primitives::LabelSetID, timeseries_type>
  void process_segment(Callback&& func) {
    Primitives::BasicTimeseries<Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<BareBones::Vector>::value_type*> timeseries;
    Primitives::SnugComposites::LabelSet::ShrinkableEncodingBimap<BareBones::Vector>::value_type last_ls;  // composite_type
    Primitives::LabelSetID last_ls_id = std::numeric_limits<Primitives::LabelSetID>::max();

    process_segment([&](Primitives::LabelSetID ls_id, Primitives::Timestamp ts, Primitives::Sample::value_type v) {
      if (ls_id != last_ls_id) {
        if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
          func(last_ls_id, timeseries);
        }

        last_ls = output_lss_[ls_id];
        timeseries.set_label_set(&last_ls);
        timeseries.samples().resize(0);
        last_ls_id = ls_id;
      }

      timeseries.samples().push_back(Primitives::Sample(ts, v));
    });

    if (last_ls_id != std::numeric_limits<Primitives::LabelSetID>::max()) {
      func(last_ls_id, timeseries);
    }
  }
};

class SegmentSamplesStorageList {
 public:
  explicit SegmentSamplesStorageList(uint64_t count) : storages_(count) {}

  class Iterator {
   public:
    using value_type = SegmentSamplesStorage::Iterator::value_type;
    using reference = value_type;
    using pointer = void;
    using difference_type = std::ptrdiff_t;
    using iterator_category = std::forward_iterator_tag;

    explicit Iterator(const Primitives::Go::Slice<SegmentSamplesStorage>& storages) : storages_(&storages) { advance(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return *it_; }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t storage_index() const noexcept { return storage_index_; }

    PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
      if (++it_ == SegmentSamplesStorage::end()) {
        ++storage_index_;
        advance();
      }

      return *this;
    }
    PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
      const auto tmp = *this;
      ++*this;
      return tmp;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE bool operator==(const BareBones::iterator::IteratorSentinelType&) const noexcept {
      return storage_index_ == storages_->size() && it_ == SegmentSamplesStorage::end();
    }

   private:
    const Primitives::Go::Slice<SegmentSamplesStorage>* storages_;
    SegmentSamplesStorage::Iterator it_{};
    uint32_t storage_index_{};

    PROMPP_ALWAYS_INLINE void advance() noexcept {
      for (; storage_index_ != storages_->size(); ++storage_index_) {
        if (const auto& storage = storages_->operator[](storage_index_); !storage.empty()) {
          it_ = storage.begin();
          break;
        }
      }
    }
  };

  [[nodiscard]] Iterator begin() const noexcept { return Iterator(storages_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static BareBones::iterator::IteratorSentinelType end() noexcept { return {}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Primitives::Go::Slice<SegmentSamplesStorage>& storages() noexcept { return storages_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Primitives::Go::Slice<SegmentSamplesStorage>& storages() const noexcept { return storages_; }

 private:
  Primitives::Go::Slice<SegmentSamplesStorage> storages_;
};

struct GoMessage {
  explicit GoMessage(const SegmentSamplesStorageList::Iterator& it) : samples_iterator(it) {}

  SegmentSamplesStorageList::Iterator samples_iterator;
  Primitives::Go::Slice<char> buffer;
  Primitives::Timestamp max_timestamp{};
  uint32_t samples_count{};
  bool delivered{};
  bool post_processed{};
};

}  // namespace PromPP::WAL

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::WAL::GoMessage> : std::true_type {};

namespace PromPP::WAL {

inline void split_messages(const SegmentSamplesStorageList& storages, uint32_t samples_count, Primitives::Go::Slice<GoMessage>& messages) {
  static const auto samples_in_series = [](const SegmentSamplesStorageList::Iterator& it) PROMPP_LAMBDA_INLINE {
    const auto& list = (*it).second;
    return list.is_single() ? 1U : list.samples().size();
  };

  if (samples_count == 0 || storages.storages().empty()) [[unlikely]] {
    return;
  }

  uint32_t message_samples_count = 0;
  for (auto it = storages.begin(); it != storages.end(); ++it) {
    if (message_samples_count == 0) [[unlikely]] {
      messages.emplace_back(it);
    }
    if (message_samples_count += samples_in_series(it); message_samples_count >= samples_count) [[unlikely]] {
      messages.back().samples_count = message_samples_count;
      message_samples_count = 0;
    }
  }
  if (message_samples_count > 0) [[likely]] {
    messages.back().samples_count = message_samples_count;
  }
}

class GoSliceSink : public snappy::Sink {
  Primitives::Go::Slice<char>& out_;

 public:
  // GoSliceSink constructor over go slice.
  PROMPP_ALWAYS_INLINE explicit GoSliceSink(Primitives::Go::Slice<char>& out) : out_(out) {}

  // Append implementation snappy::Sink.
  PROMPP_ALWAYS_INLINE void Append(const char* data, size_t len) override { out_.push_back(data, data + len); }
};

// ProtobufEncoderStats stats for encoded to snappy protobuf data.
struct ProtobufEncoderStats {
  int64_t max_timestamp{};
  size_t samples_count{};

  PROMPP_ALWAYS_INLINE bool operator==(const ProtobufEncoderStats& other) const noexcept = default;
};

class ProtobufEncoder {
 public:
  template <class LssGetter>
  void encode(const LssGetter& lss_getter, size_t message_index, size_t messages_count, std::span<GoMessage> messages) {
    assert(message_index + messages_count <= messages.size());

    for (const auto last_index = message_index + messages_count; message_index < last_index; ++message_index) {
      protobuf_.clear();

      auto& message = messages[message_index];
      create_protobuf_message(lss_getter, message);
      snappy_compress(message.buffer);
    }
  }

 private:
  static constexpr int kTimeseriesTag = 1;

  std::string protobuf_;

  template <class LssGetter>
  PROMPP_ALWAYS_INLINE void create_protobuf_message(const LssGetter& lss_getter, GoMessage& message) {
    message.max_timestamp = 0;

    protozero::pbf_writer pb_writer(protobuf_);
    protozero::basic_pbf_writer pb_timeseries(protobuf_);
    uint32_t last_ls_id = Primitives::kInvalidLabelSetID;
    uint32_t storage_index = std::numeric_limits<uint32_t>::max();
    const std::remove_reference_t<decltype(lss_getter(0))>* lss = nullptr;
    uint32_t processed_samples_count{};

    for (auto it = message.samples_iterator; it != SegmentSamplesStorageList::end(); ++it) {
      const auto [ls_id, samples_list] = *it;
      if (storage_index != it.storage_index()) [[unlikely]] {
        storage_index = it.storage_index();
        last_ls_id = Primitives::kInvalidLabelSetID;
        lss = &lss_getter(storage_index);
      }

      const auto write_sample = [&](const Primitives::Sample& sample) PROMPP_LAMBDA_INLINE {
        Prometheus::RemoteWrite::write_sample(pb_timeseries, sample);

        message.max_timestamp = std::max(message.max_timestamp, sample.timestamp());
        ++processed_samples_count;
      };

      if (last_ls_id != ls_id) [[likely]] {
        std::destroy_at(&pb_timeseries);
        std::construct_at(&pb_timeseries, pb_writer, kTimeseriesTag);

        // clang-tidy give false-positive warning on this line because lss always set in the storage_index != it.storage_index() branch
        // before the first use.
        // NOLINTNEXTLINE(clang-analyzer-core.CallAndMessage)
        Prometheus::RemoteWrite::write_label_set(pb_timeseries, lss->operator[](ls_id));
        last_ls_id = ls_id;
      }

      if (samples_list.is_single()) [[likely]] {
        write_sample(samples_list.sample());
      } else {
        for (const auto& sample : samples_list.samples()) {
          write_sample(sample);
        }
      }

      if (message.samples_count == processed_samples_count) [[unlikely]] {
        break;
      }
    }
  }

  PROMPP_ALWAYS_INLINE void snappy_compress(Primitives::Go::Slice<char>& output) const {
    GoSliceSink writer(output);
    snappy::ByteArraySource reader(protobuf_.c_str(), protobuf_.size());
    snappy::Compress(&reader, &writer);
  }
};

}  // namespace PromPP::WAL

template <>
struct BareBones::IsTriviallyReallocatable<PromPP::WAL::ProtobufEncoder> : std::true_type {};
