package task_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/stretchr/testify/require"
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
