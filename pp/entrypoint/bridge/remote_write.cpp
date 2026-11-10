#include "annotations.hpp"
#include "entrypoint/types/lss.h"
#include "wal/output_decoder.h"

/**
 * @brief Destroy the message list.
 *
 * @param args {
 *     message_list []Message
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_remote_write_message_list_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::Slice<PromPP::WAL::GoMessage> message_list;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

namespace {

using MessageEncoder = PromPP::WAL::ProtobufEncoder;

}  // namespace

/**
 * @brief Create the message encoder list.
 *
 * @param args {
 *     encoders_count uint64
 * }
 *
 * @param res {
 *     encoders []MessageEncoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_remote_write_message_encoders_ctor(void* args, void* res) {
  struct Arguments {
    uint64_t encoders_count;
  };

  using Result = struct {
    PromPP::Primitives::Go::Slice<MessageEncoder> encoders;
  };

  const auto out = static_cast<Result*>(res);
  new (&out->encoders) PromPP::Primitives::Go::Slice<MessageEncoder>(static_cast<Arguments*>(args)->encoders_count);
}

/**
 * @brief Destroy the message encoder list.
 *
 * @param args {
 *     encoders []MessageEncoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_remote_write_message_encoders_dtor(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::Slice<MessageEncoder> encoders;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Encode a remote write message.
 *
 * @param args {
 *     encoder        *MessageEncoder
 *     snapshot_list  []uintptr
 *     message_index  uint64
 *     messages_count uint64
 *     messages       []Message
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_remote_write_encode_message(void* args) {
  struct Arguments {
    MessageEncoder* encoder;
    PromPP::Primitives::Go::SliceView<entrypoint::types::SnapshotLSSVariantPtr> snapshot_list;
    uint64_t message_index;
    uint64_t messages_count;
    PromPP::Primitives::Go::SliceView<PromPP::WAL::GoMessage> messages;
  };

  const auto in = static_cast<Arguments*>(args);

  const auto snapshot_getter = [in](uint32_t shard_id) -> const entrypoint::types::SnapshotLSS& {
    return std::get<entrypoint::types::SnapshotLSS>(*in->snapshot_list[shard_id]);
  };

  in->encoder->encode(snapshot_getter, in->message_index, in->messages_count, in->messages);
}
