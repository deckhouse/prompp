#include <cstdint>
#include <cstdio>
#include <string>

#include "bare_bones/exception.h"
#include "wal/hashdex/scraper/scraper.h"

using PromPP::WAL::hashdex::scraper::Error;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;

int main() {
  // Build a buffer whose real `# TYPE ... untyped` line sits *past* the 4 GiB
  // mark. Before the 64-bit-offset fix MarkedString's uint32 offset overflowed
  // and `text()`/`metric_name()` resolved to garbage; now they must be exact.
  constexpr uint64_t kFourGiB = 4ULL * 1024 * 1024 * 1024;
  const std::string filler = "# filler comment line used only to pad the scrape buffer past four gib\n";

  std::string buf;
  buf.reserve(kFourGiB + 4096);
  while (buf.size() < kFourGiB + 1024) {
    buf += filler;
  }

  const uint64_t type_line_pos = buf.size();
  buf += "# TYPE demo_metric untyped\n";
  buf += "demo_metric{a=\"b\"} 1 123\n";

  const uint64_t untyped_abs_offset = type_line_pos + std::string_view("# TYPE demo_metric ").size();
  std::printf("buffer size           = %llu bytes (%.2f GiB)\n", (unsigned long long)buf.size(), buf.size() / double(kFourGiB) * 4);
  std::printf("'untyped' real offset = %llu (0x%llx), > 4 GiB\n", (unsigned long long)untyped_abs_offset, (unsigned long long)untyped_abs_offset);

  PrometheusScraper scraper;
  Error error{Error::kNoError};
  try {
    error = scraper.parse(buf, 0);
  } catch (const BareBones::Exception& e) {
    std::printf("caught BareBones::Exception [0x%016lx]: %.*s\n", e.code(), (int)e.message().size(), e.message().data());
    return 2;
  }
  std::printf("parse error           = %u\n", (uint32_t)error);

  int rc = 0;
  for (const auto& md : scraper.metadata()) {
    const std::string_view name = md.metric_name();
    const std::string_view text = md.text();
    const bool ok = name == "demo_metric" && text == "untyped";
    std::printf("metadata: name=\"%.*s\" type-text=\"%.*s\"  -> %s\n", (int)name.size(), name.data(), (int)text.size(), text.data(), ok ? "OK" : "CORRUPT");
    if (!ok) {
      rc = 1;
    }
  }
  return rc;
}
