# Head

## Creation

Creates shards(*LSS*, *DataStorage*, *Wal*), run goroutines of the head, stored in the **Manager**.

## Active

Head is used to add and read current data, stored in the **Manager**:

- **Appender** - add current data:
  - *LSS* - write;
  - *DataStorage* - write;
  - *Wal*(commit) - encode(LSS read) and write;
- **Querier** - provides querying access over time series data:
  - *LSS* - read;
  - *DataStorage* - read;
- **Committer**(by timer):
  - *Wal*(commit) - encode(LSS read) and write;
- **Merger**:
  - *DataStorage*(MergeOutOfOrderChunks) - write;
- **Flusher**(on rotate):
  - *Wal*(commit) - encode(LSS read) and write;
- **ActiveHeadContainer**(on shutdown) - container for active Head with weighted locker, wait all active task is finished and close semaphore with lock(on append returns error);
- **Flusher**(on shutdown):
  - *Wal*(commit) - encode(LSS read) and write;
  - *Wal*(flush) - write;
  - *Wal* close;
- **Stopper**(on shutdown) - wait all active task is finished and close query semaphore with lock(on select returns empty series set), stop goroutine, *Wal* close.

## Rotated

The head that has completed its work, but has not yet been converted into blocks, is read-only, and new data is not being added, stored in the **Keeper**:

- **Querier** - provides querying access over time series data:
  - *LSS* - read;
  - *DataStorage* - read;
- **BlockWriter** - converts the head into prom blocks and writes them to a storage:
  - *DataStorage*(MergeOutOfOrderChunks) - write;
  - **Flusher**:
    - *Wal*(flush) - write;
    - *Wal* close if flush operations were successful;
  - *WriteBlock*:
    - *LSS* - read;
    - *DataStorage* - read;
- **Flusher**(on shutdown):
  - *Wal*(flush) - write;
  - *Wal* close;
- **Stopper**(on shutdown or persist) - wait all active task is finished and close query semaphore with lock(on select returns empty series set), stop goroutine, *Wal* close.
