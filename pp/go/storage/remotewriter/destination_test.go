package remotewriter_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/remotewriter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDestinationConfigEqualTo_equal(t *testing.T) {
	const example = `
url: "http://thanos-receiver.monitoring.svc.cluster.local:19291/api/v1/receive"
bearer_token: token1
queue_config:
  capacity: 10000
  max_shards: 50
  min_shards: 1
  max_samples_per_send: 2000
  batch_send_deadline: 10s
  min_backoff: 30ms
  max_backoff: 5s`

	var cfg1, cfg2 remotewriter.DestinationConfig
	require.NoError(t, yaml.UnmarshalStrict([]byte(example), &cfg1))
	require.NoError(t, yaml.UnmarshalStrict([]byte(example), &cfg2))

	assert.True(t, cfg1.EqualTo(cfg2))
}

func TestDestinationConfigEqualTo_not_equal_secret(t *testing.T) {
	const example1 = `
url: "http://thanos-receiver.monitoring.svc.cluster.local:19291/api/v1/receive"
bearer_token: token1
queue_config:
  capacity: 10000
  max_shards: 50
  min_shards: 1
  max_samples_per_send: 2000
  batch_send_deadline: 10s
  min_backoff: 30ms
  max_backoff: 5s`
	const example2 = `
url: "http://thanos-receiver.monitoring.svc.cluster.local:19291/api/v1/receive"
bearer_token: token2
queue_config:
  capacity: 10000
  max_shards: 50
  min_shards: 1
  max_samples_per_send: 2000
  batch_send_deadline: 10s
  min_backoff: 30ms
  max_backoff: 5s`

	var cfg1, cfg2 remotewriter.DestinationConfig
	require.NoError(t, yaml.UnmarshalStrict([]byte(example1), &cfg1))
	require.NoError(t, yaml.UnmarshalStrict([]byte(example2), &cfg2))

	assert.False(t, cfg1.EqualTo(cfg2))
}
