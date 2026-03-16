#include "head/lss.h"
#include "wal/output_decoder.h"

extern "C" void prompp_remote_write_message_list_ctor(void* args, void* res) {
  struct Arguments {
    uint64_t messages_count;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<PromPP::WAL::GoMessage> message_list;
  };

  const auto out = static_cast<Result*>(res);
  new (&out->message_list) PromPP::Primitives::Go::Slice<PromPP::WAL::GoMessage>(static_cast<Arguments*>(args)->messages_count);
}

extern "C" void prompp_remote_write_message_list_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::Slice<PromPP::WAL::GoMessage> message_list;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

using MessageEncoder = PromPP::WAL::ProtobufEncoder;

extern "C" void prompp_remote_write_message_encoders_ctor(void* args, void* res) {
  struct Arguments {
    uint64_t encoders_count;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<MessageEncoder> encoders;
  };

  const auto out = static_cast<Result*>(res);
  new (&out->encoders) PromPP::Primitives::Go::Slice<MessageEncoder>(static_cast<Arguments*>(args)->encoders_count);
}

extern "C" void prompp_remote_write_message_encoders_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::Slice<MessageEncoder> encoders;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_remote_write_encode_message(void* args) {
  struct Arguments {
    MessageEncoder* encoder;
    PromPP::Primitives::Go::SliceView<entrypoint::head::LssVariantPtr> lss_list;
    PromPP::WAL::SegmentSamplesStorageList* storages;
    uint64_t message_index;
    uint64_t messages_count;
    PromPP::WAL::GoMessage* message;
  };

  const auto in = static_cast<Arguments*>(args);

  const auto lss_getter = [in](uint32_t shard_id) -> const entrypoint::head::EncodingBimap& {
    return std::get<entrypoint::head::EncodingBimap>(*in->lss_list[shard_id]);
  };

  in->encoder->encode(*in->storages, lss_getter, in->message_index, in->messages_count, *in->message);
}
