# Changelog

## v0.8.0

### Enhancements
1. **Snapshot LSS type separation.** Decoupled the read-only label set snapshot into a dedicated `SnapshotLSS` type with its own variant, reducing the active head's variant footprint and improving type safety.
2. **GOST-compliant build hardening.** Enabled `FORTIFY_SOURCE=2`, stack protector, position-independent code, and additional compiler warnings (null-dereference, division-by-zero, array-bounds) across all C++ code including third-party libraries.
3. **GCC 14 and clang-tidy 21.** Upgraded the C++ toolchain to GCC 14.2.0 and clang-tidy 21.1.8 with new `bugprone-*` diagnostics enabled; all findings resolved.
4. **Go `/sync/*` runtime metrics.** The Prometheus Go collector now exports mutex and semaphore contention statistics from `runtime/metrics` (`/sync/*`) alongside the existing GC and scheduler metrics, making locker contention observable in production.
5. **Jemalloc resident memory metric.** Exposed jemalloc's resident set size as a new metric alongside the existing allocated/mapped stats, giving operators clearer visibility into the C++ allocator's memory footprint.
6. **Chunk recoder optimization.** Switched the chunk recoder to a seek-based decode iterator and tuned the Prometheus `chunkenc` encoder with `[[likely]]` annotations, giving roughly a 10% speedup on the recoder benchmark.
7. **Remote write data source refactor.** Reworked the WAL/encoder pipeline — extracted `SegmentSamplesStorage`, added a V2 WAL reader and `DataSourceV2`, and the shard now transparently switches between WAL format versions.
8. **Merge-shard series sets.** New generic `mergeShardSeriesSet` / `mergeShardChunkSeriesSet` iterators stream across shards without an intermediate merge buffer, lowering query memory pressure on sharded heads.
9. **Jemalloc arena pool recycling.** Arenas returned to the free pool are now reset and purged instead of being destroyed, with updated jemalloc build options. New metrics report arena pool releases and reclaimed bytes (`prompp_common_jemalloc_arena_pool_*`). Carried over from v0.7.11.

### Fixes
1. **Stored XSS in React web UI (CVE-2026-40179, CVE-2026-44903, CVE-2026-44990).** Backported upstream fixes (GHSA-vffh-x6r8-xx99, GHSA-fw8g-cg8f-9j28) — graph tooltips, the Metrics Explorer fuzzy results, the heatmap bucket tooltip, and the heatmap y-axis tick labels now `escapeHTML` metric names and label values (including `le`) before injecting them into `innerHTML`. As a defense-in-depth measure for the unpatched `sanitize-html` `<xmp>` bypass (GHSA-rpr9-rxv7-x643), the Flags page now also HTML-escapes the fuzzy-search output before passing it to `sanitize-html`, so the sanitizer never sees raw markup. Together this blocks script execution from crafted metrics ingested via scrape, remote-write, or OTLP and from operator-supplied command-line flag values.
2. **OpenTelemetry security update.** Upgraded `go.opentelemetry.io/otel/sdk` and the `otlptracehttp` exporter to v1.43.0 — mitigates a PATH hijacking CVE (GHSA-hfvc-g4fc-pqhx) in the BSD host-id detector and adds a 4 MiB response body limit to OTLP HTTP exporters, protecting against memory exhaustion from a misbehaving collector.
2. **Close WAL on shard rotation.** Shard rotation now explicitly closes the outgoing WAL via a dedicated `ClosedWal` sentinel instead of leaking the handle, preventing stale WAL readers from racing with newly-rotated shards.
3. **Go 1.26.3.** Bumped Go to 1.26.3, pulling in stdlib security fixes from the 1.26.x series.
4. **aarch64 jemalloc page size.** Aligned the jemalloc build with the aarch64 host page size so ARM64 builds no longer hit a configuration mismatch under the GCC 14 toolchain.

### Other
1. **Bazel Bzlmod migration.** Migrated `pp/` to Bzlmod and refreshed `rules_cc`, `rules_foreign_cc`, and `bazel_clang_tidy` to resolve dependency conflicts that had blocked further updates of the C++ build stack.

## v0.7.11

### Enhancements
1. **Jemalloc arena pool recycling.** Arenas returned to the free pool are reset and purged instead of being destroyed, with updated jemalloc build options. New metrics report arena pool releases and reclaimed bytes (`prompp_common_jemalloc_arena_pool_*`).

### Other
1. **GCC 14 C++ toolchain.** The CI/devcontainer image and Bazel configuration on this branch build the C++ core with GCC 14.

## v0.7.10

### Fixes
1. **`highestSentTimestamp` reported in milliseconds.** The `prometheus_remote_storage_queue_highest_sent_timestamp_seconds` metric was emitted in milliseconds, causing the shard controller to compute a huge lag and falsely trigger the `PrometheusRemoteWriteDesiredShards` alert.
2. **Catalog sync with deleted records.** When reading the catalog and compacting the log, records with `deletedAt != 0` are now dropped eagerly instead of lingering in memory until the next cleanup pass.
3. **`lastPri` in the priority-weighted locker.** Fixed the `lastPri` pointer update in `util/locker/priweighted` when the tail waiter is cancelled — the priority-prefix invariant could previously be violated, leading to potential hangs.
4. **Go 1.25.9.** Bumped Go from 1.25.8 to 1.25.9, pulling in stdlib security fixes for `crypto/x509` (chain building and policy validation DoS), `crypto/tls` (TLS 1.3 KeyUpdate DoS), `html/template` (XSS via JS template literal context tracking), `archive/tar` (unbounded allocation on GNU sparse), and `os` (TOCTOU in `Root.Chmod` on Linux).
5. **npm dependency security updates.** Updated `follow-redirects` (auth header leak on cross-domain redirect) and `lodash` (prototype pollution and code injection in `_.template`) in the web UI.

## v0.7.9

### Features
1. **WAL v2 and remote write encoding.** Introduces a new WAL read/write path (v2) with refactored segment sample storage and remote-write protobuf encoding, version-aware segment handling when switching between WAL file formats, and related metrics and Go bindings updates.

### Fixes
1. **`op_top` in query strings.** Fixed PromQL string serialization for the `op_top` aggregator so expressions round-trip correctly in rules and anywhere queries are printed.
2. **Outdated corrupted head on GC.** Catalog garbage collection now removes stale corrupted head directories instead of leaving them on disk indefinitely.

### Enhancements
1. **Environment-driven defaults in configuration.** Settings that were only applied via environment variables are now folded into default configuration, aligning operator defaults with the main configuration model.

## v0.7.8

### Fixes
1. **Jemalloc VmPTE growth.** Tuned jemalloc configuration to prevent unbounded virtual address space growth when using custom arenas (create/destroy pattern). Added `retain:false`, `abort_conf:true`, and set `muzzy_decay_ms:0`, eliminating multi-GB page table overhead.
2. **gRPC authorization bypass CVE.** Updated `google.golang.org/grpc` from v1.78.0 to v1.79.3 to fix an authorization bypass via missing leading slash in `:path` (GO-2026-4762).
3. **npm dependency security update.** Updated `picomatch` to fix a high-severity ReDoS and method injection vulnerability.

## v0.7.7

### Features
1. **Custom PromQL functions.** Added `op_defined`, `op_replace_nan`, `op_smoothie`, `op_zero_if_none` window functions and `op_top` aggregation operator for operational monitoring — checking metric freshness, replacing NaN values, smoothing time series, and top-K aggregation.

### Performance
1. **Three-stage remote write parallelization.** Remote write pipeline now uses a three-stage parallel architecture, improving throughput for high-volume metric delivery.
2. **Reworked remote write encoding.** Remote write protobuf encoding redesigned with message boundary tracking and improved segment iteration, reducing encoding overhead.
3. **Jemalloc arena allocators.** DataStorage now uses jemalloc arena-based allocators with size class awareness, improving memory allocation patterns and reducing fragmentation.
4. **Composite type independence.** Refactored internal composite types (Symbol, LabelNameSet, LabelSet) to be independent of underlying filament storage, improving data access patterns.
5. **Removed unnecessary indirections.** Eliminated `std::unique_ptr` overhead from LabelSet storage and simplified the scrape adapter interface by removing the redundant `AppendScraperHashdex` method.

### Fixes
1. **OpenTelemetry PATH hijacking CVE.** Upgraded OpenTelemetry SDK to v1.40.0 to address a high-severity arbitrary code execution vulnerability via PATH hijacking.
2. **Go 1.25.8.** Updated Go from 1.25.7 to 1.25.8; the release includes security fixes for `html/template`, `os`, and `net/url`.
3. **npm dependency security updates.** Updated vulnerable npm packages in the web UI, including `immutable` (prototype pollution).

## v0.7.6

### Fixes
1. **Sample count in WAL batch segments.** Fixed incorrect sample counting when adding segments to a batch: the segment now properly returns `sampleCount` from `SamplesStorage` and resets the counter, preventing miscounted ingestion metrics.

### Features
1. **DefaultSampleAgeLimit feature flag.** Added a feature flag to control the default sample age limit, allowing operators to enable or disable the age-based sample filtering without changing global configuration.

### Enhancements
1. **Label limits applied at adapter level.** The adapter now reads label limits from the global configuration via a new `ApplyConfig` method and stores them atomically, ensuring label limits are enforced consistently without restarts. Configuration errors now cause an immediate exit to prevent silent misconfigurations.

## v0.7.5

### Fixes
1. **Platform-specific jemalloc page size for ARM64.** On ARM64 systems (e.g. Raspberry Pi 5) the kernel page size can be 16KB or 64KB, while jemalloc was hardcoded to 4KB. This caused an immediate crash with "Unsupported system page size". The build now sets the appropriate lg-page for ARM64 (64KB) and keeps 4KB for x86_64.
2. **Vector erase bug.** Fixed a bug in `BareBones::Vector::erase` that could cause incorrect behavior in optimized builds; the implementation now uses `destroy_n` / `uninitialized_default_construct_n` instead of manual loops.

### Enhancements
1. **WALGoHeadHashdex.** `WALGoHeadHashdex` has been introduced to optimize the addition of data from rules stored in the transaction head, avoiding unnecessary copying and allocation.
2. **C++ malloc metrics.** Added metrics for memory allocations from C++ code (atomic counters and optimized calculation), enabling better observability of allocator behavior.
3. **Remote write parallel encoding.** Refactored remote write message encoding: encoding from a batch to a protobuf message is now parallelized, which speeds up the encoder.
4. **Go 1.25.7.** Updated Go from 1.25.5 to 1.25.7; the release includes security fixes for crypto/tls, crypto/x509, and the go command, plus compiler fixes.

### Performance
1. **More pools.** Head and related code now use a pool provider for reusable objects, which has reduced the number of allocations.

### Other
1. **CI: workflow_dispatch and golang lint image.** Added manual trigger (`workflow_dispatch`) to the CI image build workflow and corrected the golang lint image tag (gcc-tools-x86_64 → gcc-tools-amd64) to match the built image.

## v0.7.4

### Fixes
1. **Leak tmp-for-creation dirs on conversion errors.** If an error occurs during head to historical block conversion it may lead to leaking resources on disk in form of dirs with suffix tmp-for-creation. It was fix with defer housekeeping.

### Enhancements
1. **Rebuilt record rules tracking staleness.** Rule evaluation now handles staleness more robustly and optimal with C++ bitsets instead of Go maps.
2. **Rules engine rework (recording + alerting).** Introduced a dedicated rules implementation in `pp-pkg/rules` (manager/group/origin, concurrency executor), added fixtures, and significantly expanded test coverage.
3. **Feature flags refactoring.** Simplified and consolidated feature flag wiring across components. Most
commonly used features setted by default, so no PROMPP_FEATURES environment needed in general setups.
3. **Publish container image to GHCR.** CI now publishes container images to GitHub Container Registry in addition to existing publishing flow.

### Performance
1. **Append optimizations.** Reduced allocations and overhead in the ingestion/append path (pooling sharded objects and tasks, avoiding per-task maps/closures, deferring staleness mapping allocation until rotation) and added an appender benchmark to track improvements.

### Other
1. **Decoding table migration.** Migrated/refactored decoding table and related encoding/decoding primitives, with broad updates to tests (primitives/series index/WAL) and minor internal consistency cleanups.

## v0.7.3

### Enhancements
1. **Improved Metrics Collection from C++ Components.** The mechanism for retrieving metrics from C++ code has been refined. This enhancement enables quick addition of metrics to the TSDB core, providing up-to-date information for further optimizations.
2. **High-Cardinality Metric Splitting in Federation API.** Added the ability to split high-cardinality metrics when returning them via the federation API. The protocol accumulates all metrics with the same name in memory before writing them to the output buffer, which could cause significant memory spikes. The feature flag `federation_split_families` now allows limiting the number of accumulated metrics. This slightly increases the page size but reduces memory spikes. Recommended value is 10,000.
3. **Optimized Allocations During Head Conversion and Historical Block Operations.** Allocations have been optimized during head-to-historical block conversion and when working with historical blocks. These are minor optimizations in performance-critical code paths.
4. **Refactored RemoteWrite Metrics Delivery.** The mechanism for sending metrics via the RemoteWrite protocol has been redesigned. The main change is the parallelization of data preparation and transmission, allowing metrics to be sent 100% of the time, which is critical for high data throughput. Additionally, message size calculation for large data streams has been corrected. Previously, messages could grow up to 30K samples despite the `max_samples_per_send` configuration parameter. This behavior has been fixed.

## v0.7.2

### Fixes
1. **Bound remote_write `max_sample_age` to retention.** The remote write configuration parameter `max_sample_age` is now constrained by `storage.tsdb.retention.time`. Previously, when not explicitly set, it defaulted to 30 days. That could lead to WAL files accumulating on disk if the remote target was unavailable.
2. **Remove temporary files for unloaded series.** Temporary files created when unloading series from memory are now removed. These files are never read again after being closed but can be large; removing them reduces disk usage.

### Enhancements
1. **Minor optimizations.** Various small performance and maintenance improvements.

## v0.7.1

### Fixes
1. **Fixed shard calculation in RemoteWrite.** Previously the number of shards could exceed the required amount.
2. **Encoder version check when restoring the last head.** If the WAL file encoder version differs from the current one, that WAL is considered impossible to continue and the head is rotated to create an up-to-date WAL.
3. **Fixed data race causing service crashes.** A mutex was added to linearize tasks when loading evicted series into memory across multiple shards, preventing race conditions.
4. **Skipped empty labels in OTLP Protobuf message processing.** Empty labels are now ignored during processing.
5. **Fixed shard count change application.** On-the-fly shard count changes now trigger forced rotation immediately, instead of waiting for the next scheduled rotation.
6. **Renamed TSDB state metrics.** Metrics have been renamed to align with standard vanilla Prometheus metrics.

### Enhancements
1. **Optimized allocations of cross-shard objects.** Objects are now allocated and freed together in contiguous memory.
2. **Optimized concurrent execution of recording rules.** Rules that do not depend on other rules in the group append data into a single shared batch which is added to the head once. This significantly speeds up recording rules.
3. **Reduced allocations in instant queries.** The number of allocations in instant queries, commonly used in the federate API, has been significantly reduced.
4. **Optimized sample decoding.** Removed intermediate in-memory sample storage and double-copying during decoding.
5. **Removed unused code.** This reduces the final binary size.
6. **Minor optimizations.** Various small improvements have been made to enhance overall performance.

## v0.7.0

### Fixes
1. **Improved StaleNaN Tracking.** The logic for tracking and recording StaleNaN values has been redesigned. The state can now migrate during rotation, allowing StaleNaNs to be tracked throughout the service's lifetime.
2. **Binary File Permissions in Containers.** Fixed permissions on binary files in the container. Permissions are now set to 755, allowing the container to run with a different user UID.

### Features
1. **Background Series Copy During Rotation.** Copying series data from the old head to the new one during rotation is now performed in the background, eliminating long blocking of the active head.
2. **Gradual Head Conversion to Historical Blocks.** Introduced a mechanism for gradual conversion of heads to historical blocks. Previously, all unconverted blocks were loaded into memory and converted at startup, which could lead to high resource consumption. Now, a limit is set on the number of loaded heads. New heads will evict older ones, which will only be loaded when newer heads are successfully converted and unloaded from memory.
3. **New Sample Serialization Model.** A new model for sample serialization in queries has been implemented. This allows markup code to execute outside the active head lock, moving execution to the data reading phase. In the future, we plan to eliminate data copying during queries by using shared memory instead.
4. **C++ to Go Metrics Transfer.** Added a mechanism for transferring metrics from C++ code to Go, enabling precise metric collection without locking the active head.

### Enhancements
1. **Task Execution Refactoring.** A major refactoring has been conducted to rebuild the concurrency model for task execution in hot data. The new model provides more control over operations and more extension points.
2. **Optimized Relabeling Allocations.** Relabeling allocations have been optimized to improve performance and reduce memory usage.
3. **Seek Method Optimization.** The `Seek` method of the sample iterator has been moved to C++, significantly improving its performance. This is especially noticeable in the execution time of rules.

### Other
1. **Tracy Wrapper for Benchmarking.** Introduced a wrapper around Tracy to enhance benchmarking and provide more detailed insights into code bottlenecks.

## v0.6.4

### Features
1. **Disable Block Compaction Flag.** Added a feature flag `disable_block_compaction` to disable the block compactor. This flag optimizes operation when integrating with Thanos.

## v0.6.3

### Fixes
1. **Historical Block Save Race Condition.** Fixed a race condition when saving a historical block that could cause the compactor to start reading the block before `fsync` was called on its files after writing.
2. **Compactor Scheduling Error.** Fixed an incorrect compactor scheduling bug that could lead to a slice bounds out-of-range error and service crash.
3. **Corrupted Block Marker Logic.** Improved handling of the corrupted hint. The marker can now be removed if the block is successfully read during the update of the available blocks list.

## v0.6.2

### Fixes
1. **Head Status Update During Rotation.** Fixed an issue where the head status could remain `active` if `storage.tsdb.retention` was set to zero, such as when running in agent mode. This caused the RemoteWrite loop does not transit to the next head.

## v0.6.1

### Fixes
1. **Empty Block Creation Check.** Added validation to prevent the creation of empty historical blocks during conversion under specific conditions.
2. **Handling of Corrupted Historical Blocks.** Improved handling of corrupted or empty historical blocks to prevent service crashes.
3. **Startup Error Handling.** Fixed an issue where errors occurring before the TSDB initialization could lead to a deadlock, requiring a manual process termination.

## v0.6.0

### Fixes
1. **Remove chunks data on convertion.** Prompptool now remove chunks_data on convertion vanilla wal. This files may obtain a lot of mmapped memory in runtime.

### Features
1. **Unused Data Unloading.** In most cases, queries touch only 6–8% of all series in TSDB. Other series can be unloaded to disk and loaded on demand. This feature can save up to 20% of RAM utilization and does not have a visible impact on querying unloaded series. If a series is queried by rules, it will not be unloaded. This feature is disabled by default and can be activated with the feature flag `unload_data_storage`.
2. **Omitting Out-of-Order StaleNaN Samples.** Unlike vanilla Prometheus, Prom++ allows adding out-of-order samples and overwriting existing data when timestamps match. However, this behavior conflicts with the handling of StaleNaNs, which are sometimes intentionally written over existing data or with a delay to be automatically discarded if fresher data is available. Now, the mechanism for writing to past timestamps no longer applies to StaleNaNs.

### Enhancements
1. **Scrape Parser Optimization.** A double pass process was used for scraped data: parsing and then reading parsed data with sharding samples. This allowed parsing the text once and quickly reading samples in all shards in parallel. However, it used a substantial amount of memory due to the intermediate state of parsed samples based on the source bytes buffer. In this version, new compression algorithms have been added, reducing the memory requirement by up to 10%.
2. **File Caches Reduction.** WAL files are read once and then written to only. To reduce cache pages in memory, the files are reopened with the flag `O_WRONLY` after reading. Also added a syscall `fadvise` to mark written and read pages as no longer needed. This reduces excessive caching.
3. **Dependency Updates.** Dependencies have been updated to mitigate CVEs.

## v0.5.2

### Fixes
1. **Flushing corrupted shard.** On start all heads try to convert which include flushing buffered data to disk. It may led to crashin on start if there is a corrupted not persisted head.

## v0.5.1

### Fixes
1. **Incorrect Regex Part Caching.** The matcher processing pipeline previously had legacy caching of regex parts based on pointer addresses, which led to incorrect behavior with certain regex patterns like `variant1|variant2|variant3`. This caching had no impact on performance, and thus it was removed.

## v0.5.0

1. Base Prometheus version bumped to 2.55.1. It's unlock switch from Prometheus 3.x installations to Prom++.
2. Update dependencies to mitigate CVEs.
3. Fixing potential problems found with static analysis.

## v0.4.0

### Fixes
1. **Use non-exclusive lock for head conversion.** Conversion is long operation with disk writes. It is read-only for rotated head, so queries may be done in parallel.

### Features
1. **Added feature flag `head_default_number_of_shards` to adjust the number of shards (default is 2).** Increasing the number of shards improves write operations while potentially slightly slowing down read operations and increasing memory consumption. This feature flag is temporary and will be removed in favor of automatic shard count calculation in the future.
2. **Introduced a two-stage process for series selection queries by matchers.** The first stage parses the regular expression using prefix trees from the index, which executes quickly but requires locks on the index during its execution. The second stage handles posting operations, which are resource-intensive due to data decoding and set operations on series IDs. By separating these stages, write locking time is reduced and read parallelism is increased since posting operations can use lightweight snapshot states without blocking appends.
3. **Implemented optimistic non-exclusive relabeling locks for data updates.** Since new series appear infrequently, if all data in a append operation is already cached in relabeling, that stage does not lock the series container or indexes. Exclusive locking only occurs when new data must be added. This mechanism works only when intra-shard parallelization is enabled (disabled by default).
4. **Added a mechanism for executing tasks on a specific shard instead of all shards.** This capability is essential for upcoming performance improvements.

### Enhancements
1. **Added metrics tracking the waiting time for locks and head rotations.** These metrics improve observability of internal delays and contention, enabling better diagnostics and tuning opportunities.
2. **Moved lock management inside task execution rather than across the entire task duration depending on task type.** This change can yield slight performance improvements when intra-shard parallelization is enabled by reducing unnecessary lock holding time.
3. **Small performance fixes.** In several parts of code there are bytes to string conversions. In some places it was not safe. In all places it was not optimal.
4. **Eliminate head allocations in original TSDB.** Prometheus TSDB used only as historical block querier and compactor. It is not necessary to allocate any buffers in it's head.

## v0.3.4

### Fixes
1. **Processing Several Backslashes in the End of Label Value.** Metric parser had incorrect processing of even number of backslashes at the end of label name or value.
2. **Handling Head in Querier on Rotation.** In some cases on rotation querier may have lost data on rotation.
3. **Priority Semaphore on Head.** In some specific setups exclusive tasks like reconfigure, rotate or shutdown could get stuck in lock awaiting after all normal-priority requests. We use semaphore with 2-level priority interface to push priority tasks in front of waiters queue.

## v0.3.3

### Fixes
1. **Fixed Snapshot Handling in ChunkQuerier.** Last updates led to loosing snapshots in ChunkQuerier that caused incorrect behaviour of RemoteRead API.

## v0.3.2

### Fixes
1. **Fixed Task Duplication in WAL Commits:** which was causing excessive disk access. Now, a commit task is queued only upon the first achievement of the sample limit in a WAL segment.

### Enhancements
1. **Increased the Sample Limit in WAL Segments:** The previous soft limit of 10K, hardcoded as a constant, is now converted to command-line flag with default raised to 100K.

### Features
1. **Added a Feature-flag to Disable Commits During RemoteWrite Requests.** This is an experimental flag and will be replaced with a generalized persistence level setting in the future.

## v0.3.1

### Fixes
1. **Fixed Channel Overflow and Shard Goroutine Deadlock:** A bug that caused channel overflow and deadlocks in shard goroutines has been fixed. The change ensures that tasks are added to the channel only from external goroutines, preventing these issues.
2. **Fixed Series Snapshot Memory Hanging:** We've corrected an issue where series snapshots were not getting cleared from memory due to problems with Finalizers in Go. The snapshots involved pointers to memory allocated in C++, and the garbage collector did not always trigger the Finalizer, causing memory to linger.
3. **Corrected Potential Object Retention Errors in fastCGo Calls:** There were potential errors related to object retention during fastCGo calls. While most of these were specific to test code, some could cause runtime errors in rare situations. These have now been addressed to improve stability.

### Enhancements
1. **Optimized Series Copying During Rotation:** We've made series copying during rotation much more efficient, reducing the time required by 7.5 times. To avoid pauses in the garbage collector, we're using the standard CGo mechanism for this process. Currently, this feature is under a feature flag and is being tested on select clusters to ensure stability and correctness. Once these tests are successful, we plan to enable it for all clusters.
2. **Revamped Task Execution System on Shards:** The task execution system on shards has been restructured to separate series processing from data handling. Each now operates with its own queues and locks, which is expected to boost the requests per second (RPS) for both read and write operations.
3. **New Feature Flag for Multiple Goroutines per Shard:** We've introduced a feature flag that allows running multiple goroutines per shard. This change is aimed at improving the scalability of read request handling, while still maintaining proper locking for exclusive write operations. This setup is particularly beneficial in scenarios where read requests heavily outweigh write requests. We are actively testing this feature on our clusters to determine the best concurrency levels before rolling out automatic tuning options.
4. **Optimized Internal Encoders and Decoders:** We use StreamVByte encoding in data storages. We optimize some operations inside this encoding to reduce instructions and memory jumps. This optimizations reduce CPU time by 10% on this operations.

## v0.3.0

### Enhancements
1. **Concurrent Data Ingestion**: Removed the exclusive lock during data ingestion, allowing for concurrent processing of batches. Insertion tasks are split into four sequential subtasks: relabeling, resharding new series, cache updating, and data insertion. This change speeds up insertions but may impact read performance. Future updates will focus on balancing read/write priorities.
2. **Improved Series Snapshot Management**: Redesigned snapshot handling to create new snapshots only on memory reallocation. This reduces RAM usage by ~10% and improves read request processing times. Further improvements expected with stabilized series copying during rotations.
3. **Optimized Series Insertion**: Minor optimizations for new series insertion. Noticeable 5% time savings when copying series during rotations.

## v0.2.6

### Fixes
1. **Fill Sources in meta.json**: The compactor writes the compaction.sources section in the meta.json file as a union of its parent sources. Thus, by creating blocks with empty sources, we end up making all blocks without sources. On the other hand, Thanos compactor relies on the list of sources to delete outdated blocks. Accordingly, blocks with an empty list of sources are automatically subject to deletion.

## v0.2.5

### Fixes
1. **Infinite Recursion During Head Conversion**: Fixed a bug in the logic where converting the head to a historical block could lead to infinite recursion.
2. **Memory Retention Issue in RemoteRead API**: Fixed a memory retention issue with recoded chunks during raw chunk requests via the RemoteRead API. A memory pointer was incorrectly held, allowing the garbage collector to reuse memory while it was still being accessed, potentially leading to segmentation faults.

## v0.2.4

### Fixes
1. **Feature Flag for Series Copy During Rotation**: The series copy operation during rotation has been placed behind a feature flag. This change addresses the high cost of the operation, which could temporarily render the service unavailable.

## v0.2.3

### Fixes
1. **Regular Expression Handling**: Fixed a bug in regular expression handling that occasionally led to out-of-bounds errors and crashes. The code handling regular expressions now has additional test coverage, including fuzz testing under ASAN, uncovering no further issues.

### Features
1. **Active LabelSets Copy during Rotation**: Active labelSets are now copied from the previous head during a rotation. This reduces index update load during the first scrape interval post-rotation. While the rotation itself no longer impacts resource consumption, there is a slight CPU usage spike due to the compactor running afterward.
2. **RemoteRead Support for Raw Chunk Data**: Added support for requesting raw chunk data via the RemoteRead protocol, enabling integration with external systems like Thanos. Since Prom++ encodes chunks in the active head differently from Prometheus, chunks are re-encoded upon request. Although this is not as efficient as Prometheus, it is more cost-effective than a full data unpack via RemoteRead.

### Enhancements
1. **WAL Encoding Tweaks**: The condition for selecting alternative timestamp encoding in the WAL encoder has been fixed. This generally results in a more compact WAL. Compatibility is maintained, and the previous incorrect condition caused no issues other than slightly increased disk usage.
2. **Multi-Architecture Docker Images**: Added support for building multi-architecture Docker images.
3. **WAL Encoder Cleanup**: Removed unused code from the WAL encoder, leading to a slight reduction in CPU usage.

## v0.2.2

### Fixes
1. **OTLP Handler**: Refactored the OTLP handler to resolve issues with duplicated data entries, reducing memory consumption and eliminating unnecessary conversions.

### Features
1. **Instant Query**: Introduced an Instant Query feature to optimize federation queries, which reduces CPU consumption and speeds up query processing.
2. **Refill Handler**: Added a new refill handler for improved system efficiency.
3. **Add arm64 builder**.

### Enhancements
1. **Chunk Recorder**: Optimized the Chunk Recorder to speed up block creation and reduce CPU usage during rotation.

## v0.2.1

### Fixes

1. **Bug fixed in parsing the flag for the maximum log file size**.

## v0.2.0

### Fixes

1. **User interface language switcher stuck**: Fixed an issue where the interface language switcher would not refresh display upon changing the language.
2. **Bug in head-to-historical block** conversion that caused blocks to be created with incorrect time boundaries, resulting in "overlapping blocks" log messages.

### Features

1. **Read-Only snapshots for LabelSets storage.** We've added read-only snapshots for label-sets storage, allowing retrieval without locking the current head. This enhancement should improve service throughput. Query will return a list of series IDs and a label-sets storage snapshot, while label-set extraction is handled by a request-processing goroutine on demand.

### Enhancements

1. **Unified encoder storage.** Encoders of different types are now stored within a single union container, instead of separate storages. This optimization reduces memory consumption and improves transitions between encoders. Some encoder types effectively act as pass-throughs, utilizing less than 10% of allocated memory. By unifying these encoders, we significantly reduced unused memory, achieving around a 10% memory improvement in sample storage.
2. **Improved monotonic sequence encoding.** Previously, transitioning from monotonic integer sequence encoding to a general encoder like GorillaValues required finalizing the current data chunk and starting a new one, increasing memory and operation costs. We've introduced a new encoder type that allows such transitions within a single chunk without data re-encoding and finalization. Now, a chunk is only finalized upon reaching 255 points.
3. **Simplified OutOfOrder point handling.** OutOfOrder points are now merged into readable data every 5 minutes by a Go ticker, simplifying the C++ storage code, which no longer tracks these points.
4. **Head rotation optimization.** Continuing our head rotation optimizations, we have separated the timing of converting the head into a historical block from its handover to the compactor.
5. **Sample configuration in release artifacts.** Added a example configuration to the release artifacts, allowing users to launch examples without having to copy this file from the documentation.
6. **Version 3 of the block catalog and corresponding migrations.** Version 2 is still in use. This change enables the ability to roll back to this version from future releases.
7. **Profiling metrics for the service.**

## v0.1.8

### Fixes

1. **CPU load issue during head rotation.** We addressed an issue where CPU load spiked due to aggressive index rebuilding on head rotation. The index updates have been redesigned to be lazy, meaning they now update upon request. This change has effectively reduced the CPU load.
2. **Decoupling head rotation and conversion timing.** To further reduce system load, we have staggered the timing between head rotation and the conversion of the previous head into a historical block. This staggered approach helps to maintain smoother operations.
3. **Potential concurrency bug with SharedMemory.** A bug involving potential concurrent access to SharedMemory has been fixed to ensure stable and safe operations across different processes.

## v0.1.7

### Fixes

* **Catalog corruption fixes.** Improved catalog recovery rules in cases of corruption, preventing service start-up failures.
* **Improved catalog file writing.** Now tracking the offset of the last successful write, ensuring invariant writing, and preventing potential issues during write errors.
* **Safe catalog compaction.** The catalog compaction process is now performed using a new file instead of overwriting the existing one. This change prevents data corruption if the compaction process is interrupted. The result is renamed to the original upon successful completion.
* **Handling missing blocks.** If a catalog entry references a head missing on disk, such records are now skipped, avoiding endless waiting and blocking of data sending via the RemoteWrite protocol. Exceptions are made for new and active heads, where the absence is considered temporary and requires awaiting resolution.
* **Error handling improvements.** Enhanced error wrapping and handling to reduce message length and add useful details, simplifying diagnostics and troubleshooting.
* **Handling option `trackTimestampsStaleness`.** This options has turned off setting scrape time for series w/o parsed timestamps, it leads to incorrect timstamps and errors in attempt to write WAL.
* **Disk-based retention.** Disk-based retention check now include our WAL in calculation.

### Features and enhancements

* **Sample storage optimization.** In the sample storage, if the last point is `StaleNaN`, it is now recorded using a single bit in the chunk encoder type instead of as a number. This change allows the use of cheaper encoders and avoids writing new data for completed series.
* **PromPPTool: Remove heads after convertation.** Now converted heads removed from disk by default.

## v0.1.6

### Fixes

* Parsing error when there is no trailing empty line.
* Incorrect handling of anchors in regular expressions within queries.
* Error in splitting blocks when converting wall of vanilla Prometheus.
* Error on sending data by RemoteWrite protocol when merging multiple series into one during output relabeling.
* Error leading to the creation of a new head during restart before rotation.
* Rotation error when unable to write wal to disk.
* Data omission on reading immediately after rotation.

### Features and enhancements

* Remove converted vanilla wals.
* Remove outdated corrupted heads.

## v0.1.5

### Fixes

* Taking into account the cardinality of negative matchers.
* Fixed heap buffer overflow bugs.

## v0.1.3

### Fixes

* Fix processing matchers with empty variant (i.e. label=~"(|something)")
* Fix sending empty messages with remote write protocol

### Features and enhancements

* Decompress snappy in cpp on processing remote write request
* Bump GCC version to 13
* Optimise inner structures by memory
* Update and tidy dependencies

## v0.1.2

### Fixes

* Fix startup after crash
* Fix prompptools convert our WAL to blocks time bounds

### Features and enhancements

* Optimized memory on RemoteWrite
* Reduce iops with commit window

## v0.1.1

### Features and enhancements

* Minor memory usage optimization
* Added metric queried_series (number of requested series by caller)

## v0.1.0

### Features and enhancements

* Enable Remote Write
* Accelerate startup
* Minor CPU and memory optimisations
