# Head

- Create;
  - create shard(lss, datastorage, wal);
  - run goroutine;
- Active:
  - Append:
    - LSS - write;
    - DataStorage - write;
    - Wal - commit(write to wal);
  - Query:
    - LSS - read;
    - DataStorage - read;
  -
- Rotated;
- Close;
- Shutdown:
  - Wal - commit(write to wal);
  - Wal - flush;
