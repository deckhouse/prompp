# Head

## Creation

Creates shards(**LSS**, **DataStorage**, **Wal**), run goroutines of the head, stored in the **Manager**.

## Active

Head is used to append and read current data, stored in the **Manager**:

- **Appender** - add current data:
  - *Append*:
    - **LSS** - write;
    - **DataStorage** - write;
    - **Wal** via task:
      - *Commit* - encode(LSS read) segment and add to segment writer(buffer);
      - *Flush* - write to storage from buffer if exist;
- **Querier** - provides querying access over time series data:
  - **LSS** - read;
  - **DataStorage** - read;
- **Manager**:
  - *MergeOutOfOrderChunks*:
    - **DataStorage**:
      - *MergeOutOfOrderChunks* - write;
  - *CommitToWal* by timer:
    - **Wal** via task:
      - *Commit* - encode(LSS read) segment and add to segment writer(buffer);
      - *Flush* - write to storage from buffer if exist;
  - *Rotate* by timer:
    - **DataStorage** via task:
      - *MergeOutOfOrderChunks* - write;
    - **Wal** via range:
      - *Commit* - encode(LSS read) segment and add to segment writer(buffer);
      - *Flush* - write to storage from buffer if exist;
  - *Shutdown*:
    - **ActiveHeadContainer** - container for active Head with weighted locker:
      - *Close* - wait all active task is finished and close semaphore with lock(on append returns error);
    - **Wal** via range:
      - *Commit* - encode(LSS read) segment and add to segment writer(buffer);
      - *Flush* - write to storage from buffer if exist;
    - **Head**:
      - *Close* - wait all active task is finished and close query semaphore with lock(on select returns empty series set), stop goroutine, **Wal** close.

## Rotated

The head that has completed its work, but has not yet been converted into blocks, is read-only, and new data is not being added, stored in the **Keeper**:

- **Querier** - provides querying access over time series data:
  - **LSS** - read;
  - **DataStorage** - read;
- **Keeper**:
  - *Write*:
    - **Wal** via range:
      - *Flush* - write to storage from buffer if exist;
      - *Close* - if flush operations were successful;
    - **BlockWriter** - converts the head into prom blocks and writes them to a storage:
      - *WriteBlock*:
        - **LSS** - read;
        - **DataStorage** - read;
  - *Shutdown*:
    - **Wal** via range:
      - *Flush* - write to storage from buffer if exist;
    - **Head**:
      - *Close* - wait all active task is finished and close query semaphore with lock(on select returns empty series set), stop goroutine, **Wal** close.
