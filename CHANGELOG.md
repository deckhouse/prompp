# Changelog

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
