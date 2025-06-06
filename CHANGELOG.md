# Changelog

## v0.3.1

### Fixes

1. **Fixed Channel Overflow and Shard Goroutine Deadlock:** A bug that caused channel overflow and deadlocks in shard goroutines has been fixed. The change ensures that tasks are added to the channel only from external goroutines, preventing these issues.
2. **Fixed Series Snapshot Memory Hanging:** We've corrected an issue where series snapshots were not getting cleared from memory due to problems with Finalizers in Go. The snapshots involved pointers to memory allocated in C++, and the garbage collector did not always trigger the Finalizer, causing memory to linger.
3. **Corrected Potential Object Retention Errors in fastCGo Calls:** There were potential errors related to object retention during fastCGo calls. While most of these were specific to test code, some could cause runtime errors in rare situations. These have now been addressed to improve stability.

### Enhancements

1. **Optimized Series Copying During Rotation:** We've made series copying during rotation much more efficient, reducing the time required by 7.5 times. To avoid pauses in the garbage collector, we're using the standard CGo mechanism for this process. Currently, this feature is under a feature flag and is being tested on select clusters to ensure stability and correctness. Once these tests are successful, we plan to enable it for all clusters.
2. **Revamped Task Execution System on Shards:** The task execution system on shards has been restructured to separate series processing from data handling. Each now operates with its own queues and locks, which is expected to boost the requests per second (RPS) for both read and write operations.
3. **New Feature Flag for Multiple Goroutines per Shard:** We've introduced a feature flag that allows running multiple goroutines per shard. This change is aimed at improving the scalability of read request handling, while still maintaining proper locking for exclusive write operations. This setup is particularly beneficial in scenarios where read requests heavily outweigh write requests. We are actively testing this feature on our clusters to determine the best concurrency levels before rolling out automatic tuning options.

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
