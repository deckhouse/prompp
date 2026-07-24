package task_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

func TestTaskWaiter(t *testing.T) {
	tw := task.NewTaskWaiter[*task.Generic[*testShard]](5)
	err := tw.Wait()
	require.NoError(t, err)
}

// testHead implementation [Shard].
type testShard struct{}

// ShardID implementation [Shard].
func (*testShard) ShardID() uint16 {
	return 0
}
