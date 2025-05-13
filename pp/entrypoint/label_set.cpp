#include "label_set.h"

#include "entrypoint/head/lss.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

using entrypoint::head::LssVariantPtr;
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

void prompp_label_set_serialize(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    Slice<Label> label_set;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) {
        auto in_label_set = lss[in->series_id];
        auto& out_label_set = out->label_set;
        out_label_set.reserve(in_label_set.size());
        std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                               [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
      },
      *in->lss);
}

extern "C" void prompp_label_set_free(void* args) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Label> label_set;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

static constexpr uint8_t kLabelSeparator = '\xFE';
static constexpr uint8_t kNameValueSeparator = '\xFF';

extern "C" void prompp_label_set_bytes_size(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    uint32_t series_id;
  };
  struct Result {
    uint32_t size;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result{.size = sizeof(kLabelSeparator)};

  std::visit(
      [in, out](auto& lss) {
        const auto& label_set = lss[in->series_id];
        for (auto label : label_set) {
          out->size += label.first.size() + label.second.size();
        }

        out->size += sizeof(kNameValueSeparator) * label_set.size() * 2 - 1;
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
  auto bytes = static_cast<Result*>(res)->bytes.data();

  *bytes++ = kLabelSeparator;

  std::visit(
      [in, &bytes](auto& lss) {
        uint32_t count = 0;
        for (auto label : lss[in->series_id]) {
          if (++count > 1) [[likely]] {
            *bytes++ = kNameValueSeparator;
          }

          std::memcpy(bytes, label.first.data(), label.first.size());
          bytes += label.first.size();

          *bytes++ = kNameValueSeparator;

          std::memcpy(bytes, label.second.data(), label.second.size());
          bytes += label.second.size();
        }
      },
      *in->lss);
}