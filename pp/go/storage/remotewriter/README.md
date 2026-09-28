1. Create cursor.
2. Create client.
3. Loop:
    1. create batch. (readTimeout)
        1. read next segment.
            1. if permanent error - batch completed + end of block is reached.
            2. if not permanent error - if batch is fulfilled or deadline reached - batch is completed.
                1. recalculate number of output shards.
            3. if no error, and batch is not full, wait for 5 sec and repeat
    2. try go (write cache). (retry+backoff)
    3. encode protobuf.
    4. send. (retry+backoff)
        1. if outdated
            1. return permanent error
        2. on error -> non permanent error
        3. on success -> nil
    5. try ack.
    6. check end of block

## Delivery marks

Every message POST carries two headers, both set from the delivery marks in the request context
(see `delivery.go`):

- `X-Idempotency-Key` — `<destination>/<headID>/<targetSegmentID>/<messageIndex>`. Stays the same
  over all the retries of that message, so a receiver can recognize a replay.
- `Retry-Attempt` — the attempt number of our own send loop, `0` for the first delivery. Set by the
  upstream `remote.Client` and only present when it is greater than zero.

The key is stable within the life of an iterator. After a restart the same segments may be cut into
different batches (`targetSegmentID` and the number of shards float), so the keys of the re-read data
may differ from those of the first run.

### Replays we do not count

`X-Idempotency-Key` is not only a mark for the receiver: `net/http` treats a POST carrying
`Idempotency-Key` or `X-Idempotency-Key` as replayable (`Request.isReplayable`), so `http.Transport`
resends it on its own when a reused connection fails — a stale keep-alive, a server closing an idle
connection, an HTTP/2 refused stream. Such a replay happens inside a single `Client.Do`, below our
round tripper, and goes out byte for byte identical: the same key **and** the same `Retry-Attempt`.

Therefore:

- the idempotency key is the only unit of deduplication, the pair of key and attempt is not unique;
- `Retry-Attempt` is a diagnostic of our send loop, not a count of times the payload reached the receiver;
- `sent_message_duration_seconds` covers all the transport replays of a send as one observation, and
  `bytes_total` undercounts what actually went over the wire.

The `connections_total{state="new"|"reused"}` metric makes the wire side visible: it counts every
connection a request went through (`httptrace.GotConn`), so transport replays show up as extra
connections for a single send.
