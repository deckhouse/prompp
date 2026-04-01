#include "label_set.h"

#include "bare_bones/algorithm.h"
#include "bare_bones/iterator.h"
#include "entrypoint/head/lss.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

using entrypoint::head::LssVariantPtr;
using entrypoint::head::SnapshotLSSVariantPtr;
using PromPP::Primitives::Go::Slice;
using PromPP::Primitives::Go::SliceView;

void prompp_label_set_length(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    size_t length;
  };

  auto in = static_cast<Arguments*>(args);

  std::visit([in, res](auto& lss) { new (res) Result{.length = lss[in->series_id].size()}; }, *in->lss);
}

void prompp_label_set_serialize_from_snapshot(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    SnapshotLSSVariantPtr snapshot;
    uint32_t series_id;
  };
  struct Result {
    Slice<Label> label_set;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& snapshot) {
        auto in_label_set = snapshot[in->series_id];
        auto& out_label_set = out->label_set;
        out_label_set.reserve(in_label_set.size());
        std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                               [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
      },
      *in->snapshot);
}

extern "C" void prompp_label_set_free(void* args) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Label> label_set;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

namespace Bytes {

static constexpr uint8_t kLabelSeparator = '\xFE';
static constexpr uint8_t kNameValueSeparator = '\xFF';

class SizeCalculator {
 public:
  template <class Label>
  PROMPP_ALWAYS_INLINE SizeCalculator& operator=(const Label& label) noexcept {
    operator()(label);
    return *this;
  }

  template <class Label>
  PROMPP_ALWAYS_INLINE void operator()(const Label& label) noexcept {
    label_size_ += label.first.size() + label.second.size();
    ++label_count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept {
    uint32_t size = sizeof(kLabelSeparator);
    if (label_count_ > 0) {
      size += label_size_ + sizeof(kNameValueSeparator) * label_count_ * 2 - 1;
    }

    return size;
  }

 private:
  uint32_t label_size_{};
  uint32_t label_count_{};
};

class Writer {
 public:
  explicit Writer(uint8_t* bytes) : bytes_(bytes) { *bytes_++ = kLabelSeparator; }

  template <class Label>
  PROMPP_ALWAYS_INLINE Writer& operator=(const Label& label) noexcept {
    operator()(label);
    return *this;
  }

  template <class Label>
  PROMPP_ALWAYS_INLINE void operator()(const Label& label) noexcept {
    if (++label_count_ > 1) [[likely]] {
      *bytes_++ = kNameValueSeparator;
    }

    std::memcpy(bytes_, label.first.data(), label.first.size());
    bytes_ += label.first.size();

    *bytes_++ = kNameValueSeparator;

    std::memcpy(bytes_, label.second.data(), label.second.size());
    bytes_ += label.second.size();
  }

  PROMPP_ALWAYS_INLINE uint32_t written_bytes(const uint8_t* start_bytes) const noexcept { return static_cast<uint32_t>(bytes_ - start_bytes); }

 private:
  uint8_t* bytes_;
  uint32_t label_count_{};
};

};  // namespace Bytes

struct LabelNameLess {
  using Label = const std::pair<std::string_view, std::string_view>;

  PROMPP_ALWAYS_INLINE bool operator()(const Label& label, const PromPP::Primitives::Go::String& name) const noexcept {
    return label.first < static_cast<std::string_view>(name);
  }
  PROMPP_ALWAYS_INLINE bool operator()(const Label& a, const Label& b) const noexcept { return a.first < b.first; }
  PROMPP_ALWAYS_INLINE bool operator()(const PromPP::Primitives::Go::String& name, const Label& label) const noexcept {
    return static_cast<std::string_view>(name) < label.first;
  }
  PROMPP_ALWAYS_INLINE bool operator()(const PromPP::Primitives::Go::String& a, const PromPP::Primitives::Go::String& b) const noexcept { return a < b; }
};

extern "C" void prompp_label_set_bytes_size(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    uint32_t size;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        Bytes::SizeCalculator calculator;
        std::ranges::for_each(lss[in->series_id], std::ref(calculator));
        out->size = calculator.size();
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data());
        std::ranges::for_each(lss[in->series_id], std::ref(writer));
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes_with_labels(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    SliceView<PromPP::Primitives::Go::String> names;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data());
        std::ranges::set_intersection(lss[in->series_id], in->names, BareBones::iterator::OperationIterator(writer), LabelNameLess{});
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}

extern "C" void prompp_label_set_bytes_without_labels(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
    SliceView<PromPP::Primitives::Go::String> names;
  };
  struct Result {
    SliceView<uint8_t> bytes;
  };

  auto in = static_cast<Arguments*>(args);
  auto& bytes = static_cast<Result*>(res)->bytes;

  std::visit(
      [in, &bytes](auto& lss) {
        Bytes::Writer writer(bytes.data());
        std::ranges::set_difference(lss[in->series_id], in->names, BareBones::iterator::OperationIterator(writer), LabelNameLess{});
        bytes.reset_to(bytes.data(), writer.written_bytes(bytes.data()), bytes.capacity());
      },
      *in->lss);
}
