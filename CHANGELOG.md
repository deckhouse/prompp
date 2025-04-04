# Changelog

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
