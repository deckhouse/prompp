package remotewriter

import (
	"errors"
	"io"
)

var (
	// ErrShardIsCorrupted error when the shard file was corrupted.
	ErrShardIsCorrupted = errors.New("shard is corrupted")
	// ErrEndOfBlock error indicating the end of the block.
	ErrEndOfBlock = errors.New("end of block")
	// ErrEmptyReadResult an error indicating an empty reading result.
	ErrEmptyReadResult = errors.New("empty read result")
)

// CloseAll closes all given closers.
func CloseAll(closers ...io.Closer) error {
	var err error
	for _, closer := range closers {
		if closer != nil {
			err = errors.Join(err, closer.Close())
		}
	}
	return err
}
