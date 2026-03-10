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
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

//
// ShardError
//

// ShardError error reading the shard.
type ShardError struct {
	err         error
	headID      string
	shardID     uint16
	processable bool
}

// NewShardError init new [ShardError].
func NewShardError(headID string, shardID uint16, processable bool, err error) ShardError {
	return ShardError{
		err:         err,
		headID:      headID,
		shardID:     shardID,
		processable: processable,
	}
}

// Error returns error as string, implementation error.
func (e ShardError) Error() string {
	return e.err.Error()
}

// HeadID returns head ID.
func (e ShardError) HeadID() string {
	return e.headID
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
	headID            string
	shardID           uint16
	corrupted         bool
	lastReadSegmentID optional.Optional[uint32]
	walReader         ShardWalReader
	segment           Segment
	decoder           *Decoder
	unclaimedSegment  *DecodedSegment
	decoderStateFile  io.WriteCloser
	segmentSize       prometheus.Histogram
}

// createShard creates a new [shard].
// If an error occurs during initialization, try to create a decoder with a reset state.
func createShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	segmentSize prometheus.Histogram,
) (*shard, error) {
	s, err := newShard(
		headID,
		shardID,
		shardFileName,
		decoderStateFileName,
		resetDecoderState,
		externalLabels,
		relabelConfigs,
		segmentSize,
	)
	if err != nil {
		logger.Errorf("failed to create shard: %v", err)
		return newShard(
			headID,
			shardID,
			shardFileName,
			decoderStateFileName,
			true,
			externalLabels,
			relabelConfigs,
			segmentSize,
		)
	}
	return s, nil
}

// newCorruptedShard creates a new corrupted [shard].
func newCorruptedShard(
	headID string,
	shardID uint16,
	segmentSize prometheus.Histogram,
) *shard {
	return &shard{
		headID:      headID,
		shardID:     shardID,
		corrupted:   true,
		segmentSize: segmentSize,
	}
}

// newShard init new [shard].
//
//nolint:dupl // this is constructor.
//revive:disable-next-line:function-length // this is constructor.
//revive:disable-next-line:flag-parameter // this is a flag, but it's more convenient this way
func newShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	segmentSize prometheus.Histogram,
) (*shard, error) {
	wr, err := newWalReader(shardFileName)
	if err != nil {
		// the shard can be restored
		return nil, fmt.Errorf("failed to create wal file reader: %w", err)
	}

	segment, err := wr.EmptySegment()
	if err != nil {
		_ = wr.Close() // it doesn't make sense for us to process this erro
		logger.Errorf("shard %s/%d is corrupted by init segment: %v", headID, shardID, err)
		// return the corrupted shard that won't be read
		return newCorruptedShard(headID, shardID, segmentSize), nil
	}

	decoder, err := NewDecoder(
		externalLabels,
		relabelConfigs,
		shardID,
		wr.EncoderVersion(),
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

	// create new shard
	return &shard{
		headID:           headID,
		shardID:          shardID,
		walReader:        wr,
		segment:          segment,
		decoder:          decoder,
		decoderStateFile: decoderStateFile,
		segmentSize:      segmentSize,
	}, nil
}

// Close closes internal shard resources - [ShardWalReader] and decoderStateFile, rendering it unusable for I/O.
func (s *shard) Close() (err error) {
	// a corrupted shard has no open walReader
	if s.walReader != nil {
		err = errors.Join(err, s.walReader.Close())
	}

	// a corrupted shard has no open decoderStateFile
	if s.decoderStateFile != nil {
		err = errors.Join(err, s.decoderStateFile.Close())
	}

	return err
}

// IsCorrupted returns true if the shard is corrupted.
func (s *shard) IsCorrupted() bool {
	return s.corrupted
}

// LSS returns the [cppbridge.LabelSetStorage] of the [shard].
func (s *shard) LSS() *cppbridge.LabelSetStorage {
	if s.decoder == nil {
		return cppbridge.NewLssStorage()
	}

	return s.decoder.lss
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

	return s.readUntil(ctx, targetSegmentID, minTimestamp, samplesStorage)
}

// WriteTo writes output decoder state to io.Writer.
func (s *shard) WriteTo(w io.Writer) (int64, error) {
	if s.corrupted {
		return 0, nil // corrupted shard doesn't write to cache
	}

	return s.decoder.WriteTo(w)
}

// readUntil reads [DecodedSegment] from wal until the target segment ID is reached.
func (s *shard) readUntil(
	ctx context.Context,
	targetSegmentID uint32,
	minTimestamp int64,
	samplesStorage *cppbridge.CppSegmentSamplesStorage,
) (*DecodedSegment, error) {
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

//
// ShardRotatedWalReader
//

// ShardRotatedWalReader a shard rotated wall reader.
type ShardRotatedWalReader interface {
	// Close wal file.
	Close() error

	// EmptySegment creates an empty segment of the required version.
	EmptySegment() (Segment, error)

	// IsEOF returns true if the rotated wal file is at the end.
	IsEOF() bool

	// ReadSegmentBody reads [Segment] body from wal.
	// It may return a non-nil error if some error condition is known, such as EOF.
	ReadSegmentBody(s Segment) error

	// ReadSegmentID reads [Segment] ID from wal.
	// It may return a non-nil error if some error condition is known, such as EOF.
	ReadSegmentID(s Segment) error
}

//
// shardRotated
//

type shardRotated struct {
	headID           string
	shardID          uint16
	corrupted        bool
	completed        bool
	walReader        ShardRotatedWalReader
	segment          Segment
	decoder          *Decoder
	decoderStateFile io.WriteCloser
	segmentSize      prometheus.Histogram
}

// createShardRotated creates a new [shardRotated].
// If an error occurs during initialization, try to create a decoder with a reset state.
func createShardRotated(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	segmentSize prometheus.Histogram,
) (*shardRotated, error) {
	s, err := newShardRotated(
		headID,
		shardID,
		shardFileName,
		decoderStateFileName,
		resetDecoderState,
		externalLabels,
		relabelConfigs,
		segmentSize,
	)
	if err != nil {
		logger.Errorf("failed to create shardRotated: %v", err)
		return newShardRotated(
			headID,
			shardID,
			shardFileName,
			decoderStateFileName,
			true,
			externalLabels,
			relabelConfigs,
			segmentSize,
		)
	}
	return s, nil
}

// newCorruptedShardRotated creates a new corrupted [shardRotated].
func newCorruptedShardRotated(
	headID string,
	shardID uint16,
	segmentSize prometheus.Histogram,
) *shardRotated {
	return &shardRotated{
		headID:      headID,
		shardID:     shardID,
		corrupted:   true,
		segmentSize: segmentSize,
	}
}

// newShardRotated init new [shardRotated].
//
//nolint:dupl // this is constructor.
//revive:disable-next-line:function-length // this is constructor.
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func newShardRotated(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	segmentSize prometheus.Histogram,
) (*shardRotated, error) {
	wr, err := newWalReaderRotated(shardFileName)
	if err != nil {
		logger.Errorf("shard %s/%d is corrupted by create wal file rotated reader: %v", headID, shardID, err)
		// return the corrupted shard that won't be read
		return newCorruptedShardRotated(headID, shardID, segmentSize), nil
	}

	segment, err := wr.EmptySegment()
	if err != nil {
		_ = wr.Close() // it doesn't make sense for us to process this error
		logger.Errorf("shard %s/%d is corrupted by init segment: %v", headID, shardID, err)
		// return the corrupted shard that won't be read
		return newCorruptedShardRotated(headID, shardID, segmentSize), nil
	}

	decoder, err := NewDecoder(
		externalLabels,
		relabelConfigs,
		shardID,
		wr.EncoderVersion(),
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

	// create new shard
	return &shardRotated{
		headID:           headID,
		shardID:          shardID,
		walReader:        wr,
		segment:          segment,
		decoder:          decoder,
		decoderStateFile: decoderStateFile,
		segmentSize:      segmentSize,
	}, nil
}

// Close closes internal shard resources - [ShardWalReader] and decoderStateFile, rendering it unusable for I/O.
func (s *shardRotated) Close() (err error) {
	// a corrupted shard has no open walReader
	if s.walReader != nil {
		err = errors.Join(err, s.walReader.Close())
	}

	// a corrupted shard has no open decoderStateFile
	if s.decoderStateFile != nil {
		err = errors.Join(err, s.decoderStateFile.Close())
	}

	return err
}

// IsCorrupted returns true if the shard is corrupted.
func (s *shardRotated) IsCorrupted() bool {
	return s.corrupted
}

// LSS returns the [cppbridge.LabelSetStorage] of the [shardRotated].
func (s *shardRotated) LSS() *cppbridge.LabelSetStorage {
	if s.decoder == nil {
		return cppbridge.NewLssStorage()
	}

	return s.decoder.lss
}

// ReadSegment reads [DecodedSegment] from wal.
func (s *shardRotated) ReadSegment(
	minTimestamp int64,
	samplesStorage *cppbridge.CppSegmentSamplesStorage,
) (*DecodedSegment, error) {
	if s.corrupted {
		return nil, ErrShardIsCorrupted
	}

	if s.completed {
		return nil, ErrEndOfBlock // no more segments in the wal file
	}

	defer s.segment.Reset()
	if err := s.walReader.ReadSegmentBody(s.segment); err != nil {
		s.corrupted = true
		logger.Errorf("remotewrite shard: %s/%d is corrupted by read body: %v", s.headID, s.shardID, err)
		return nil, errors.Join(err, ErrShardIsCorrupted)
	}
	s.segmentSize.Observe(float64(s.segment.Length()))

	decodedSegment, err := s.decoder.Decode(s.segment.Bytes(), minTimestamp, samplesStorage)
	if err != nil {
		s.corrupted = true
		logger.Errorf("remotewrite shard: %s/%d is corrupted by decode: %v", s.headID, s.shardID, err)
		return nil, errors.Join(err, ErrShardIsCorrupted)
	}
	decodedSegment.ID = s.segment.ID()

	return decodedSegment, nil
}

// SegmentID returns the ID of the [Segment] that can be read from wal.
// It may return a non-nil error if some error condition is known, such as EOF.
func (s *shardRotated) SegmentID() (uint32, error) {
	if s.corrupted {
		return reader.UnknownSegmentID, ErrShardIsCorrupted
	}

	if s.segment.ID() != reader.UnknownSegmentID {
		return s.segment.ID(), nil
	}

	if s.walReader.IsEOF() {
		s.completed = true
		return reader.UnknownSegmentID, ErrEndOfBlock // no more segments in the wal file
	}

	err := s.walReader.ReadSegmentID(s.segment)
	if err == nil {
		return s.segment.ID(), nil
	}

	if errors.Is(err, io.EOF) {
		s.completed = true
		return reader.UnknownSegmentID, ErrEndOfBlock // no more segments in the wal file
	}

	s.corrupted = true

	logger.Errorf("remotewrite shard: %s/%d is corrupted by read ID: %v", s.headID, s.shardID, err)
	return reader.UnknownSegmentID, errors.Join(err, ErrShardIsCorrupted)
}

// SkipSegment it reads and skips the [Segment] from the wal.
func (s *shardRotated) SkipSegment(
	minTimestamp int64,
	samplesStorage *cppbridge.CppSegmentSamplesStorage,
) error {
	if s.corrupted {
		return ErrShardIsCorrupted
	}

	if s.completed {
		return ErrEndOfBlock // no more segments in the wal file
	}

	defer s.segment.Reset()
	if err := s.walReader.ReadSegmentBody(s.segment); err != nil {
		s.corrupted = true
		logger.Errorf("remotewrite shard: %s/%d is corrupted by read body: %v", s.headID, s.shardID, err)
		return errors.Join(err, ErrShardIsCorrupted)
	}
	s.segmentSize.Observe(float64(s.segment.Length()))

	if _, err := s.decoder.Decode(s.segment.Bytes(), minTimestamp, samplesStorage); err != nil {
		s.corrupted = true
		logger.Errorf("remotewrite shard: %s/%d is corrupted by decode: %v", s.headID, s.shardID, err)
		return errors.Join(err, ErrShardIsCorrupted)
	}

	cppbridge.ClearSegmentSamplesStorage(samplesStorage)

	return nil
}

// WriteTo writes output decoder state to io.Writer.
func (s *shardRotated) WriteTo(w io.Writer) (int64, error) {
	if s.corrupted {
		return 0, nil // corrupted shard doesn't write to cache
	}

	return s.decoder.WriteTo(w)
}
