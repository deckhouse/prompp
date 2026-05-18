package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/storage/remote"
	"golang.org/x/sync/semaphore"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out
//go:generate moq mock/protobuf_writer.go . ProtobufWriter
//go:generate moq mock/target_segment_id_set_closer.go . TargetSegmentIDSetCloser

//
// DataSourceV2
//

// DataSourceV2 a data source of the head shards for sending data through the RemoteWriter..
type DataSourceV2 interface {
	// Close write caches and closes the data source and releases the resources.
	Close() error

	// Init it initializes the data source by reading segments from shards until the required number is reached.
	Init(ctx context.Context, targetSegmentID uint32) error

	// LSSSnapshots returns the snapshots of the label set storages,
	// it's used to create message encoders, creating from shard decoder lss snapshots.
	LSSSnapshots() []*cppbridge.LabelSetSnapshot

	// Next checks the segmentID for readiness and reads the [DecodedSegment] from the shards.
	Next(
		ctx context.Context,
		minTimestamp int64,
		segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
	) ([]*DecodedSegment, error)

	// NumberOfLSSes returns the number of label set storages.
	NumberOfLSSes() int

	// WriteCaches writes caches to the buffer and sends the signal to write the caches.
	WriteCaches()
}

//
// TargetSegmentIDSetCloser
//

// TargetSegmentIDSetCloser is a implementation of target segment id set closer.
type TargetSegmentIDSetCloser interface {
	SetTargetSegmentID(segmentID uint32) error
	Close() error
}

//
// ProtobufWriter
//

// ProtobufWriter is a implementation of protobuf writer.
type ProtobufWriter interface {
	Write(ctx context.Context, data []byte) error
}

type sharder struct {
	min            int
	max            int
	numberOfShards int
}

// newSharder creates a new [sharder].
func newSharder(minShards, maxShards int) (*sharder, error) {
	if minShards > maxShards || minShards <= 0 {
		return nil, fmt.Errorf("failed to create sharder, min: %d, max: %d", minShards, maxShards)
	}
	return &sharder{
		min:            minShards,
		max:            maxShards,
		numberOfShards: minShards,
	}, nil
}

// Apply applies the value for the number of shards to the sharder.
func (s *sharder) Apply(value float64) {
	newValue := int(math.Ceil(value))
	if newValue < s.min {
		newValue = s.min
	} else if newValue > s.max {
		newValue = s.max
	}

	s.numberOfShards = newValue
}

// BestNumberOfShards clamping value between min and max.
func (s *sharder) BestNumberOfShards(value float64) int {
	newValue := int(math.Ceil(value))
	if newValue < s.min {
		newValue = s.min
	} else if newValue > s.max {
		newValue = s.max
	}

	return newValue
}

// NumberOfShards returns the number of shards.
func (s *sharder) NumberOfShards() int {
	return s.numberOfShards
}

// Iterator is a iterator for sending data to the remote storage.
type Iterator struct {
	clock                    clockwork.Clock
	queueConfig              config.QueueConfig
	dataSource               DataSourceV2
	protobufWriter           ProtobufWriter
	targetSegmentIDSetCloser TargetSegmentIDSetCloser
	metrics                  *DestinationMetrics
	targetSegmentID          uint32

	outputSharder *sharder

	scrapeInterval    time.Duration
	endOfBlockReached bool
}

// newIterator creates a new [Iterator].
func newIterator(
	clock clockwork.Clock,
	queueConfig config.QueueConfig,
	dataSource DataSourceV2,
	targetSegmentIDSetCloser TargetSegmentIDSetCloser,
	targetSegmentID uint32,
	readTimeout time.Duration,
	protobufWriter ProtobufWriter,
	metrics *DestinationMetrics,
) (*Iterator, error) {
	outputSharder, err := newSharder(queueConfig.MinShards, queueConfig.MaxShards)
	if err != nil {
		return nil, err
	}

	return &Iterator{
		clock:                    clock,
		queueConfig:              queueConfig,
		dataSource:               dataSource,
		protobufWriter:           protobufWriter,
		targetSegmentIDSetCloser: targetSegmentIDSetCloser,
		metrics:                  metrics,
		targetSegmentID:          targetSegmentID,
		scrapeInterval:           readTimeout,
		outputSharder:            outputSharder,
	}, nil
}

// wrapError wraps the error.
func (i *Iterator) wrapError(err error) error {
	if err != nil {
		return err
	}

	if i.endOfBlockReached {
		return ErrEndOfBlock
	}

	return nil
}

// Next reads data from the data source and writes it to the protobuf writer.
//
//revive:disable-next-line:function-length // long but readable
//revive:disable-next-line:cyclomatic // long but readable
//revive:disable-next-line:cognitive-complexity // long but readable
func (i *Iterator) Next(ctx context.Context) (*batch, error) { //revive:disable-line:unexported-return
	if i.endOfBlockReached {
		return nil, i.wrapError(nil)
	}

	startTime := i.clock.Now()
	var delay time.Duration
	numberOfShards := i.outputSharder.NumberOfShards()
	i.metrics.numShards.Set(float64(numberOfShards))
	b := newBatch(i.dataSource.NumberOfLSSes(), numberOfShards, i.queueConfig.MaxSamplesPerSend)
	deadline := i.clock.After(i.scrapeInterval)

readLoop:
	for {
		select {
		case <-ctx.Done():
			return nil, i.wrapError(ctx.Err())
		case <-deadline:
			break readLoop
		case <-i.clock.After(delay):
		}

		readStartTime := i.clock.Now()
		decodedSegments, err := i.dataSource.Next(ctx, i.minTimestamp(), b.segmentSampleStorages)
		i.metrics.readSegmentDuration.Observe(i.clock.Since(readStartTime).Seconds())

		if err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				i.endOfBlockReached = true
				break readLoop
			}

			if errors.Is(err, ErrEmptyReadResult) {
				delay = defaultDelay
				continue
			}

			logger.Errorf("datasource read failed: %v", err)
			delay = defaultDelay
			continue
		}

		b.add(decodedSegments)
		i.setTargetSegmentID(decodedSegments)

		if b.IsFilled() {
			break readLoop
		}

		delay = 0
	}

	generateStartTime := i.clock.Now()
	readDuration := generateStartTime.Sub(startTime)

	if b.HasDroppedSamples() {
		i.metrics.droppedSamplesTotal.WithLabelValues(reasonTooOld).Add(float64(b.OutdatedSamplesCount()))
		i.metrics.droppedSamplesTotal.WithLabelValues(reasonDroppedSeries).Add(float64(b.DroppedSamplesCount()))
	}

	i.metrics.droppedSeriesTotal.Add(float64(b.DroppedSeriesCount()))

	if b.IsEmpty() {
		return nil, i.wrapError(nil)
	}

	i.metrics.generateBatchDuration.Observe(readDuration.Seconds())
	i.metrics.addSeriesTotal.Add(float64(b.AddSeriesCount()))

	// Ideal number of shards is when batch contains all samples from scrape interval
	// so we can predict number of samples per scrape interval as
	// scrapeInterval / readDuration * numberOfSamplesInBatch
	// next step is to divide it by max samples per shard to get desired number of shards.
	desiredNumberOfShards := float64(i.scrapeInterval) / float64(readDuration) *
		float64(b.NumberOfSamples()) / float64(b.MaxNumberOfSamplesPerShard())

	numberOfMessages := math.Ceil(float64(b.NumberOfSamples()) / float64(b.MaxNumberOfSamplesPerShard()))
	bestNumberOfShards := i.outputSharder.BestNumberOfShards(numberOfMessages)

	i.outputSharder.Apply(desiredNumberOfShards)
	i.metrics.desiredNumShards.Set(desiredNumberOfShards)
	i.metrics.bestNumShards.Set(float64(bestNumberOfShards))

	i.writeCaches()

	b.snapshots = i.dataSource.LSSSnapshots()
	b.targetSegmentID = i.targetSegmentID
	return b, nil
}

// EncodeBatch encodes the batch into a message list and records encode duration metric.
// It is intended to be called from the encode stage of the write pipeline.
func (i *Iterator) EncodeBatch(b *batch) *cppbridge.RWMessageList {
	defer func(encodeStartTime time.Time) {
		i.metrics.encodeBatchDuration.Observe(i.clock.Since(encodeStartTime).Seconds())
	}(i.clock.Now())

	encodersCount := b.numberOfShards
	messages := b.segmentSampleStorages.SplitMessages(
		uint32(b.maxNumberOfSamplesPerShard), // #nosec G115 // no overflow
		b.TargetSegmentID(),
	)
	encoders := cppbridge.NewMessageEncoders(
		uint64(encodersCount), // #nosec G115 // no overflow
		b.snapshots,
	)

	messagesCount := len(messages.Messages)
	messagesPerEncoder := messagesCount / encodersCount
	if messagesPerEncoder == 0 {
		messagesPerEncoder = 1
	}

	wg := sync.WaitGroup{}
	for messageIndex, encoderIndex := 0, 0; messageIndex < messagesCount; encoderIndex++ {
		var encodeCount int
		if encoderIndex+1 == encodersCount {
			encodeCount = messagesCount - messageIndex
		} else {
			encodeCount = messagesPerEncoder
		}

		wg.Add(1)
		go func(encoderIndex, messageIndex, encodeCount int) {
			defer wg.Done()

			encoders.Encode(
				encoderIndex,
				uint64(messageIndex), // #nosec G115 // no overflow
				uint64(encodeCount),  // #nosec G115 // no overflow
				messages.Messages,
			)
		}(encoderIndex, messageIndex, encodeCount)

		messageIndex += encodeCount
	}
	wg.Wait()

	runtime.KeepAlive(b)
	messages.UpdateStats()
	return messages
}

// SendMessage sends the message to the remote storage.
// It is intended to be called from the send stage of the write pipeline.
func (i *Iterator) SendMessage(ctx context.Context, msg *cppbridge.RWMessageList) error {
	i.metrics.samplesTotal.Add(float64(msg.NumberOfSamples()))
	sendersCount := i.outputSharder.max

	sendIteration := 0
	err := backoff.Retry(func() error {
		defer func() { sendIteration++ }()
		if msg.IsObsoleted(i.minTimestamp()) {
			for _, messageShard := range msg.Messages {
				if messageShard.Delivered {
					continue
				}
				i.metrics.droppedSamplesTotal.WithLabelValues(reasonTooOld).Add(float64(messageShard.SampleCount))
			}
			return nil
		}

		sendSemaphore := semaphore.NewWeighted(int64(sendersCount))
		startTime := i.clock.Now()
		for index := range msg.Messages {
			if msg.Messages[index].Delivered {
				continue
			}

			if err := sendSemaphore.Acquire(ctx, 1); err != nil {
				break
			}

			go func(msg *cppbridge.RWMessage) {
				defer sendSemaphore.Release(1)
				sendStartTime := i.clock.Now()
				writeErr := i.protobufWriter.Write(ctx, msg.Buffer)
				if writeErr != nil {
					logger.Errorf("failed to send protobuf: %v", writeErr)
				}
				i.metrics.sentMessageDuration.Observe(time.Since(sendStartTime).Seconds())

				msg.Delivered = !errors.As(writeErr, &remote.RecoverableError{})
			}(&msg.Messages[index])
		}
		_ = sendSemaphore.Acquire(ctx, int64(sendersCount))
		i.metrics.sentBatchDuration.Observe(i.clock.Since(startTime).Seconds())

		var failedSamplesTotal uint64
		var sentBytesTotal uint64
		var highestSentTimestamp int64
		var retriedSamplesTotal uint64
		for _, shrd := range msg.Messages {
			if shrd.Delivered {
				if shrd.PostProcessed {
					continue
				}
				// delivered on this iteration
				shrd.PostProcessed = true
				retriedSamplesTotal += uint64(shrd.SampleCount)
				sentBytesTotal += uint64(len(shrd.Buffer))
				if highestSentTimestamp < shrd.MaxTimestamp {
					highestSentTimestamp = shrd.MaxTimestamp
				}
				continue
			}
			// delivery failed bool
			retriedSamplesTotal += uint64(shrd.SampleCount)
			failedSamplesTotal += uint64(shrd.SampleCount)
		}

		i.metrics.failedSamplesTotal.Add(float64(failedSamplesTotal))
		i.metrics.sentBytesTotal.Add(float64(sentBytesTotal))
		i.metrics.highestSentTimestamp.Set(float64(highestSentTimestamp) / 1000)

		if sendIteration > 0 {
			i.metrics.retriedSamplesTotal.Add(float64(retriedSamplesTotal))
		}

		if msg.HasDataToDeliver() {
			return errors.New("not all data delivered")
		}

		return nil
	},
		backoff.WithContext(
			backoff.NewExponentialBackOff(
				backoff.WithClockProvider(i.clock),
				backoff.WithMaxElapsedTime(0),
				backoff.WithMaxInterval(i.scrapeInterval),
			),
			ctx,
		),
	)
	if err != nil {
		return i.wrapError(err)
	}

	if err = i.tryAck(msg.TargetSegmentID); err != nil {
		logger.Errorf("failed to ack segment id: %v", err)
		return i.wrapError(nil)
	}

	return nil
}

// setTargetSegmentID sets the target segment id.
func (i *Iterator) setTargetSegmentID(decodedSegments []*DecodedSegment) {
	for _, segment := range decodedSegments {
		i.targetSegmentID = max(i.targetSegmentID, segment.ID+1)
	}
}

func (i *Iterator) writeCaches() {
	i.dataSource.WriteCaches()
}

func (i *Iterator) tryAck(targetSegmentID uint32) error {
	if targetSegmentID == 0 {
		return nil
	}

	if err := i.targetSegmentIDSetCloser.SetTargetSegmentID(targetSegmentID); err != nil {
		return fmt.Errorf("failed to set target segment id: %w", err)
	}

	return nil
}

func (i *Iterator) minTimestamp() int64 {
	sampleAgeLimit := time.Duration(i.queueConfig.SampleAgeLimit)
	return i.clock.Now().Add(-sampleAgeLimit).UnixMilli()
}

// Close closes the iterator.
func (i *Iterator) Close() error {
	return errors.Join(i.dataSource.Close(), i.targetSegmentIDSetCloser.Close())
}

// batch is a accumulate samples from decoded segments.
type batch struct {
	snapshots                  []*cppbridge.LabelSetSnapshot
	segmentSampleStorages      *cppbridge.SegmentSamplesStorageList
	numberOfShards             int
	numberOfSamples            int
	maxNumberOfSamplesPerShard int
	outdatedSamplesCount       uint32
	droppedSamplesCount        uint32
	addSeriesCount             uint32
	droppedSeriesCount         uint32
	targetSegmentID            uint32
}

// newBatch creates a new [batch].
func newBatch(numberOfHeadShards, numberOfShards, maxNumberOfSamplesPerShard int) *batch {
	return &batch{
		numberOfShards: numberOfShards,
		segmentSampleStorages: cppbridge.NewSegmentSamplesStorage(
			uint64(numberOfHeadShards), // #nosec G115 // no overflow
		),
		maxNumberOfSamplesPerShard: maxNumberOfSamplesPerShard,
	}
}

// add adds the samples stats to the batch.
func (b *batch) add(segments []*DecodedSegment) {
	for _, segment := range segments {
		b.numberOfSamples += int(segment.SampleCount)
		b.outdatedSamplesCount += segment.OutdatedSamplesCount
		b.droppedSamplesCount += segment.DroppedSamplesCount
		b.addSeriesCount += segment.AddSeriesCount
		b.droppedSeriesCount += segment.DroppedSeriesCount
	}
}

// IsFilled checks if the batch is filled.
func (b *batch) IsFilled() bool {
	return b.numberOfSamples > b.numberOfShards*b.maxNumberOfSamplesPerShard
}

// IsEmpty checks if the batch is empty.
func (b *batch) IsEmpty() bool {
	return b.numberOfSamples == 0
}

// HasDroppedSamples checks if the batch has dropped samples.
func (b *batch) HasDroppedSamples() bool {
	return b.droppedSamplesCount > 0 || b.outdatedSamplesCount > 0
}

// OutdatedSamplesCount number of outdated samples.
func (b *batch) OutdatedSamplesCount() uint32 {
	return b.outdatedSamplesCount
}

// DroppedSamplesCount number of dropped samples.
func (b *batch) DroppedSamplesCount() uint32 {
	return b.droppedSamplesCount
}

// AddSeriesCount number of add series.
func (b *batch) AddSeriesCount() uint32 {
	return b.addSeriesCount
}

// DroppedSeriesCount number of dropped series.
func (b *batch) DroppedSeriesCount() uint32 {
	return b.droppedSeriesCount
}

// NumberOfSamples the total number of samples.
func (b *batch) NumberOfSamples() int {
	return b.numberOfSamples
}

// MaxNumberOfSamplesPerShard the maximum number of samples per shard.
func (b *batch) MaxNumberOfSamplesPerShard() int {
	return b.maxNumberOfSamplesPerShard
}

// NumberOfShards the number of shards.
func (b *batch) NumberOfShards() int {
	return b.numberOfShards
}

// TargetSegmentID the target segment id.
func (b *batch) TargetSegmentID() uint32 {
	return b.targetSegmentID
}
