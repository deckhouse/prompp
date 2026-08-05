#include <cstdio>
#include <fstream>
#include <string>

#include "wal/hashdex/scraper/scraper.h"

using PromPP::WAL::hashdex::scraper::Error;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;

namespace {

const char* error_name(Error e) {
  switch (e) {
    case Error::kNoError:
      return "kNoError";
    case Error::kUnexpectedToken:
      return "kUnexpectedToken";
    case Error::kNoMetricName:
      return "kNoMetricName";
    case Error::kInvalidUtf8:
      return "kInvalidUtf8";
    case Error::kInvalidValue:
      return "kInvalidValue";
    case Error::kInvalidTimestamp:
      return "kInvalidTimestamp";
    case Error::kMarkupBufferOverflow:
      return "kMarkupBufferOverflow";
  }
  return "kUnknown";
}

}  // namespace

int main(int argc, char** argv) {
  int rc = 0;
  for (int i = 1; i < argc; ++i) {
    std::ifstream t(argv[i]);
    std::string str((std::istreambuf_iterator<char>(t)), std::istreambuf_iterator<char>());

    PrometheusScraper scraper;
    const auto error = scraper.parse(str, 0);

    std::printf("%-24s error=%-18s metrics=%u metadata=%u\n", argv[i], error_name(error), scraper.floats().size(), scraper.metadata().size());
    if (error != Error::kNoError) {
      rc = 1;
    }
  }
  return rc;
}
