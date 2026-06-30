#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief destroy message list
 *
 * @param args {
 *     message_list []Message
 * }
 */
void prompp_remote_write_message_list_dtor(void* args);

/**
 * @brief create message encoders list
 *
 * @param args {
 *     encodersCount uint64
 * }
 *
 * @param res {
 *     encoders []MessageEncoder
 * }
 */
void prompp_remote_write_message_encoders_ctor(void* args, void* res);

/**
 * @brief destroy message encoders list
 *
 * @param args {
 *     encoders []MessageEncoder
 * }
 */
void prompp_remote_write_message_encoders_dtor(void* args);

/**
 * @brief encode remote write message
 *
 * @param args {
 *     encoder        *MessageEncoder
 *     lss_list       []uintptr
 *     messageIndex   uint64
 *     messagesCount  uint64
 *     messages       []Message
 * }
 *
 */
void prompp_remote_write_encode_message(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
