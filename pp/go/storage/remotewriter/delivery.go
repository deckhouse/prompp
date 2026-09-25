package remotewriter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"
)

// IdempotencyKeyHeader marks a remote write POST, so that a receiver is able to recognize its replays.
// Replaying a POST is a regular part of the delivery contract here: [Iterator.SendMessage] resends
// undelivered messages of a batch with the very same payload.
//
// Mind that this exact header name also opts the request into replays made by the Go transport itself:
// [net/http.Request.isReplayable] treats a POST carrying Idempotency-Key or X-Idempotency-Key as
// replayable, so http.Transport silently resends it when a reused connection fails (a stale keep-alive,
// a server closing an idle connection, an HTTP/2 refused stream). Those replays happen below our round
// tripper, inside a single Client.Do, so they go out byte for byte identical - the same key and the same
// Retry-Attempt value. Hence the delivery contract for a receiver:
//   - the idempotency key is the only unit of deduplication, a pair of key and attempt is NOT unique;
//   - [RetryAttemptHeader] counts the attempts of our own send loop, not the times a payload reached
//     the receiver, and is meant for diagnostics.
const IdempotencyKeyHeader = "X-Idempotency-Key"

// RetryAttemptHeader is set by [remote.Client] from the attempt number [protobufWriter] passes to Store.
const RetryAttemptHeader = "Retry-Attempt"

// deliveryMarks describes a single delivery attempt of a single message.
type deliveryMarks struct {
	idempotencyKey string
	retryAttempt   int
}

// deliveryMarksContextKey is a key of the [deliveryMarks] value in a request context.
type deliveryMarksContextKey struct{}

// withDeliveryMarks returns a context carrying the marks of a single message delivery attempt.
// The key travels to [IdempotencyKeyHeader] via [deliveryRoundTripper], the attempt number
// travels to [RetryAttemptHeader] via [protobufWriter].
func withDeliveryMarks(ctx context.Context, idempotencyKey string, retryAttempt int) context.Context {
	return context.WithValue(
		ctx,
		deliveryMarksContextKey{},
		deliveryMarks{idempotencyKey: idempotencyKey, retryAttempt: retryAttempt},
	)
}

// deliveryMarksFromContext returns the delivery marks from the context, a zero value if there are none.
func deliveryMarksFromContext(ctx context.Context) deliveryMarks {
	marks, _ := ctx.Value(deliveryMarksContextKey{}).(deliveryMarks)
	return marks
}

// deliveryTarget describes where an [Iterator] delivers messages to.
type deliveryTarget struct {
	// endpoint is the redacted destination URL, it tells the logs where a message was not delivered to.
	endpoint string
	// idempotencyKeyPrefix is the head-wide part of a message idempotency key.
	idempotencyKeyPrefix string
}

// newDeliveryTarget init new [deliveryTarget] of the destination reading the head headID.
func newDeliveryTarget(destinationName, endpoint, headID string) deliveryTarget {
	return deliveryTarget{
		endpoint:             endpoint,
		idempotencyKeyPrefix: destinationName + "/" + headID,
	}
}

// messageIdempotencyKey builds the key of a single message of a batch. Messages of a batch are
// numbered by messageIndex and a non-empty batch always advances targetSegmentID, so the key
// identifies the message within the head and stays the same over all its retries.
func (t deliveryTarget) messageIdempotencyKey(targetSegmentID uint32, messageIndex int) string {
	return fmt.Sprintf("%s/%d/%d", t.idempotencyKeyPrefix, targetSegmentID, messageIndex)
}

// deliveryRoundTripper sets [IdempotencyKeyHeader] from the request context and counts the connections
// the requests go through.
type deliveryRoundTripper struct {
	http.RoundTripper
	trace *httptrace.ClientTrace
}

// newDeliveryRoundTripper init new [deliveryRoundTripper] over the underlying round tripper.
func newDeliveryRoundTripper(
	underlyingRT http.RoundTripper,
	metrics *DestinationMetrics,
) *deliveryRoundTripper {
	newConnections := metrics.connectionsTotal.WithLabelValues(connectionNew)
	reusedConnections := metrics.connectionsTotal.WithLabelValues(connectionReused)

	return &deliveryRoundTripper{
		RoundTripper: underlyingRT,
		// a single trace is shared by all the requests, it only touches the counters
		trace: &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				if info.Reused {
					reusedConnections.Inc()
					return
				}

				newConnections.Inc()
			},
		},
	}
}

// RoundTrip implementation [http.RoundTripper].
func (rt *deliveryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if key := deliveryMarksFromContext(req.Context()).idempotencyKey; key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}

	// the transport may replay the request under a single RoundTrip, every attempt of it reports GotConn
	return rt.RoundTripper.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), rt.trace)))
}
