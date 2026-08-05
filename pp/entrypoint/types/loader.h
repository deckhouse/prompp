#pragma once

#include <cstdint>
#include <span>

#include "bare_bones/iterator.h"
#include "bare_bones/preprocess.h"
#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/lss.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/reverter.h"

namespace entrypoint::types {

class RevertableLoader {
 public:
  RevertableLoader(DataStorageWithArenas& storage,
                   QueryableEncodingBimap::LsIdSetIterator ls_id_begin,
                   QueryableEncodingBimap::LsIdSetIterator ls_id_end,
                   uint32_t ls_id_batch_size)
      : loader_(storage), reverter_(storage), iterator_(ls_id_begin, ls_id_batch_size), end_iterator_(ls_id_end) {
    add_series();
  }

  bool next_batch() {
    iterator_.next_batch();
    if (iterator_ != end_iterator_) {
      add_series();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void load_next(std::span<const uint8_t> buffer) { loader_.load_next(buffer); }
  PROMPP_ALWAYS_INLINE void load_finalize() { loader_.load_finalize(); }

  PROMPP_ALWAYS_INLINE void revert() { reverter_.revert(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE DataStorageWithArenas& storage() noexcept { return loader_.storage(); }

 private:
  ::series_data::unloading::Loader<DataStorageWithArenas> loader_;
  ::series_data::unloading::LoadReverter<DataStorageWithArenas> reverter_;
  BareBones::iterator::BatchIterator<QueryableEncodingBimap::LsIdSetIterator, QueryableEncodingBimap::LsIdSetIterator> iterator_;
  [[no_unique_address]] QueryableEncodingBimap::LsIdSetIterator end_iterator_;

  void add_series() {
    loader_.add_series_to_load(iterator_, end_iterator_, iterator_.batch_size());
    reverter_.add_series_to_revert<decltype(iterator_)&>(iterator_, end_iterator_, iterator_.batch_size());
  }
};

}  // namespace entrypoint::types
