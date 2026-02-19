package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

//
// ShardError
//

// ShardError error reading the shard.
type ShardError struct {
	shardID     uint16
	processable bool
	err         error
}

// NewShardError init new [ShardError].
func NewShardError(shardID uint16, processable bool, err error) ShardError {
	return ShardError{
		shardID:     shardID,
		processable: processable,
		err:         err,
	}
}

// Error returns error as string, implementation error.
func (e ShardError) Error() string {
	return e.err.Error()
}

// ShardID returns shard ID.
func (e ShardError) ShardID() uint16 {
	return e.shardID
}

// Unwrap retruns source error.
func (e ShardError) Unwrap() error {
	return e.err
}

//
// ShardWalReader
//

// ShardWalReader a shard wall reader.
type ShardWalReader interface {
	// Close wal file.
	Close() error

	// EmptySegment creates an empty segment of the required version.
	EmptySegment() (Segment, error)

	// Read reads up data into s [Segment] from wal.
	// It may return a non-nil error if some error condition is known, such as EOF.
	Read(s Segment) error
}

// NoOpShardWalReader a shard wall reader, do nothing.
type NoOpShardWalReader struct{}

// Close implementation [ShardWalReader], do nothing.
func (NoOpShardWalReader) Close() error { return nil }

// Read implementation [ShardWalReader], do nothing.
func (NoOpShardWalReader) Read() (segment Segment, err error) { return segment, io.EOF }

//
// shard
//

type shard struct {
	headID             string
	shardID            uint16
	corrupted          bool
	lastReadSegmentID  optional.Optional[uint32]
	walReader          ShardWalReader
	segment            Segment
	decoder            *Decoder
	unclaimedSegment   *DecodedSegment
	decoderStateFile   io.WriteCloser
	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

// newShard init new [shard].
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func newShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*shard, error) {
	wr, encoderVersion, err := newWalReader(shardFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create wal file reader: %w", err)
	}

	decoder, err := NewDecoder(
		externalLabels,
		relabelConfigs,
		shardID,
		encoderVersion,
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to create decoder: %w", err), wr.Close())
	}

	decoderStateFileFlags := os.O_CREATE | os.O_RDWR
	if resetDecoderState {
		decoderStateFileFlags |= os.O_TRUNC
	}
	decoderStateFile, err := os.OpenFile( // #nosec G304 // it's meant to be that way
		decoderStateFileName,
		decoderStateFileFlags,
		0o600, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to open decoder state file: %w", err), wr.Close())
	}

	if !resetDecoderState {
		if err = decoder.LoadFrom(decoderStateFile); err != nil {
			return nil, errors.Join(
				fmt.Errorf("failed to restore from cache: %w", err), wr.Close(), decoderStateFile.Close(),
			)
		}
	} else {
		if err = decoderStateFile.Truncate(0); err != nil {
			return nil, errors.Join(
				fmt.Errorf("failed to truncate decoder state file: %w", err), wr.Close(), decoderStateFile.Close(),
			)
		}
	}

	segment, err := wr.EmptySegment()
	if err != nil {
		_ = wr.Close() // it doesn't make sense for us to process this erro
		logger.Errorf("shard %s/%d is corrupted by init: %v", headID, shardID, err)
		// return the corrupted shard that won't be read
		return &shard{
			headID:             headID,
			shardID:            shardID,
			corrupted:          true,
			decoder:            decoder,
			decoderStateFile:   decoderStateFile,
			unexpectedEOFCount: unexpectedEOFCount,
			segmentSize:        segmentSize,
		}, nil
	}

	// create new shard
	return &shard{
		headID:             headID,
		shardID:            shardID,
		walReader:          wr,
		segment:            segment,
		decoder:            decoder,
		decoderStateFile:   decoderStateFile,
		unexpectedEOFCount: unexpectedEOFCount,
		segmentSize:        segmentSize,
	}, nil
}

// Close closes internal shard resources - [ShardWalReader] and decoderStateFile, rendering it unusable for I/O.
func (s *shard) Close() (err error) {
	// a corrupted shard has no open walReader
	if s.walReader != nil {
		err = errors.Join(err, s.walReader.Close())
	}

	return errors.Join(err, s.decoderStateFile.Close())
}

// Read [Segment] from WAL and decode to [DecodedSegment].
// Discards segments with a lower ID than the targetSegmentID and returns them only if they are equal.
func (s *shard) Read(
	ctx context.Context,
	targetSegmentID uint32,
	minTimestamp int64,
	samplesStorage *cppbridge.CppSegmentSamplesStorage,
) (*DecodedSegment, error) {
	if s.corrupted {
		return nil, ErrShardIsCorrupted
	}

	// if the reader has read a segment whose ID is greater than the required ID, we will defer it until it is requested
	if s.unclaimedSegment != nil && s.unclaimedSegment.ID == targetSegmentID {
		decodedSegment := s.unclaimedSegment
		s.unclaimedSegment = nil
		return decodedSegment, nil
	}

	if !s.lastReadSegmentID.IsNil() && s.lastReadSegmentID.Value() >= targetSegmentID {
		return nil, nil
	}

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		s.segment.Reset()
		if err := s.walReader.Read(s.segment); err != nil {
			s.corrupted = true
			logger.Errorf("remotewritedebug shard %s/%d is corrupted by read: %v", s.headID, s.shardID, err)
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}
		s.segmentSize.Observe(float64(s.segment.Length()))

		decodedSegment, err := s.decoder.Decode(s.segment.Bytes(), minTimestamp, samplesStorage)
		if err != nil {
			s.corrupted = true
			logger.Errorf("remotewritedebug shard %s/%d is corrupted by decode: %v", s.headID, s.shardID, err)
			return nil, errors.Join(err, ErrShardIsCorrupted)
		}

		decodedSegment.ID = s.segment.ID()
		s.lastReadSegmentID.Set(decodedSegment.ID)

		if decodedSegment.ID == targetSegmentID {
			return decodedSegment, nil
		}

		if decodedSegment.ID > targetSegmentID {
			s.unclaimedSegment = decodedSegment
			return nil, nil
		}

		cppbridge.ClearSegmentSamplesStorage(samplesStorage)
	}
}

func (s *shard) SetCorrupted() {
	s.corrupted = true
}
