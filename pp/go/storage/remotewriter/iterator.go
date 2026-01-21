package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"math"
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

//
// DataSource
//

// DataSource is a implementation of data source.
type DataSource interface {
	Read(
		ctx context.Context,
		targetSegmentID uint32,
		minTimestamp int64,
		segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
	) ([]*DecodedSegment, error)
	LSSes() []*cppbridge.LabelSetStorage
	NumberOfLSSes() int
	WriteCaches()
	Close() error
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
	clock                        clockwork.Clock
	queueConfig                  config.QueueConfig
	dataSource                   DataSource
	protobufWriter               ProtobufWriter
	targetSegmentIDSetCloser     TargetSegmentIDSetCloser
	metrics                      *DestinationMetrics
	targetSegmentID              uint32
	targetSegmentIsPartiallyRead bool

	outputSharder *sharder

	scrapeInterval    time.Duration
	endOfBlockReached bool
}

// newIterator creates a new [Iterator].
func newIterator(
	clock clockwork.Clock,
	queueConfig config.QueueConfig,
	dataSource DataSource,
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
func (i *Iterator) Next(ctx context.Context) (*cppbridge.RWMessageList, error) {
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
		decodedSegments, err := i.dataSource.Read(ctx, i.targetSegmentID, i.minTimestamp(), b.segmentSampleStorages)
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
		i.targetSegmentID++
		i.targetSegmentIsPartiallyRead = false

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
	desiredNumberOfShards := float64(i.scrapeInterval) / float64(readDuration) * float64(b.NumberOfSamples()) / float64(b.MaxNumberOfSamplesPerShard())

	numberOfMessages := math.Ceil(float64(b.NumberOfSamples()) / float64(b.MaxNumberOfSamplesPerShard()))
	bestNumberOfShards := i.outputSharder.BestNumberOfShards(numberOfMessages)

	i.outputSharder.Apply(desiredNumberOfShards)
	i.metrics.desiredNumShards.Set(desiredNumberOfShards)
	i.metrics.bestNumShards.Set(float64(bestNumberOfShards))

	i.writeCaches()

	encodeStartTime := i.clock.Now()
	msg := i.encode(b, int(numberOfMessages)) // #nosec G115 // no overflow
	i.metrics.encodeBatchDuration.Observe(i.clock.Since(encodeStartTime).Seconds())

	return msg, nil
}

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
				retriedSamplesTotal += shrd.SampleCount
				sentBytesTotal += uint64(len(shrd.Buffer))
				if highestSentTimestamp < shrd.MaxTimestamp {
					highestSentTimestamp = shrd.MaxTimestamp
				}
				continue
			}
			// delivery failed bool
			retriedSamplesTotal += shrd.SampleCount
			failedSamplesTotal += shrd.SampleCount
		}

		i.metrics.failedSamplesTotal.Add(float64(failedSamplesTotal))
		i.metrics.sentBytesTotal.Add(float64(sentBytesTotal))
		i.metrics.highestSentTimestamp.Set(float64(highestSentTimestamp))

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

	if err = i.tryAck(ctx); err != nil {
		logger.Errorf("failed to ack segment id: %v", err)
	}

	return i.wrapError(nil)
}

func (i *Iterator) writeCaches() {
	i.dataSource.WriteCaches()
}

func (i *Iterator) encode(batch *batch, numberOfMessages int) *cppbridge.RWMessageList {
	encodersCount := batch.numberOfShards

	messages := cppbridge.NewRWMessageList(uint64(numberOfMessages))
	encoders := cppbridge.NewMessageEncoders(uint64(encodersCount), i.dataSource.LSSes())
	messagesPerEncoder := numberOfMessages / encodersCount
	if messagesPerEncoder == 0 {
		messagesPerEncoder = 1
	}

	wg := sync.WaitGroup{}
	for messageIndex, encoderIndex := 0, 0; messageIndex < numberOfMessages; encoderIndex++ {
		var encodeCount int
		if encoderIndex+1 == encodersCount {
			encodeCount = numberOfMessages - messageIndex
		} else {
			encodeCount = messagesPerEncoder
		}

		wg.Add(1)
		go func(encoderIndex, messageIndex, encodeCount int) {
			defer wg.Done()

			for ; encodeCount > 0; messageIndex++ {
				encoders.Encode(
					encoderIndex,
					batch.segmentSampleStorages,
					uint64(messageIndex),
					uint64(numberOfMessages),
					&messages.Messages[messageIndex],
				)
				encodeCount--
			}
		}(encoderIndex, messageIndex, encodeCount)

		messageIndex += encodeCount
	}
	wg.Wait()

	messages.UpdateStats()
	return messages
}

func (i *Iterator) tryAck(_ context.Context) error {
	if i.targetSegmentID == 0 && i.targetSegmentIsPartiallyRead {
		return nil
	}

	targetSegmentID := i.targetSegmentID
	if i.targetSegmentIsPartiallyRead {
		targetSegmentID--
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

type batch struct {
	segments                   []*DecodedSegment
	segmentSampleStorages      *cppbridge.SegmentSamplesStorageList
	numberOfShards             int
	numberOfSamples            int
	outdatedSamplesCount       uint32
	droppedSamplesCount        uint32
	addSeriesCount             uint32
	droppedSeriesCount         uint32
	maxNumberOfSamplesPerShard int
}

// newBatch creates a new [batch].
func newBatch(numberOfHeadShards, numberOfShards, maxNumberOfSamplesPerShard int) *batch {
	return &batch{
		numberOfShards:             numberOfShards,
		segmentSampleStorages:      cppbridge.NewSegmentSamplesStorage(uint64(numberOfHeadShards)),
		maxNumberOfSamplesPerShard: maxNumberOfSamplesPerShard,
	}
}

func (b *batch) add(segments []*DecodedSegment) {
	for _, segment := range segments {
		b.numberOfSamples += int(segment.SampleCount)
		b.outdatedSamplesCount += segment.OutdatedSamplesCount
		b.droppedSamplesCount += segment.DroppedSamplesCount
		b.addSeriesCount += segment.AddSeriesCount
		b.droppedSeriesCount += segment.DroppedSeriesCount
	}
}

func (b *batch) IsFilled() bool {
	return b.numberOfSamples > b.numberOfShards*b.maxNumberOfSamplesPerShard
}

func (b *batch) IsEmpty() bool {
	return b.numberOfSamples == 0
}

func (b *batch) HasDroppedSamples() bool {
	return b.droppedSamplesCount > 0 || b.outdatedSamplesCount > 0
}

func (b *batch) OutdatedSamplesCount() uint32 {
	return b.outdatedSamplesCount
}

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

func (b *batch) NumberOfSamples() int {
	return b.numberOfSamples
}

func (b *batch) MaxNumberOfSamplesPerShard() int {
	return b.maxNumberOfSamplesPerShard
}

func (b *batch) NumberOfShards() int {
	return b.numberOfShards
}

func (b *batch) Data() []*DecodedSegment {
	return b.segments
}
