# Changelog

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
