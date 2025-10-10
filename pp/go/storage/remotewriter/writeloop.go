package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/storage/remote"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

const defaultDelay = time.Second * 5

type writeLoop struct {
	dataDir       string
	destination   *Destination
	currentHeadID *string
	catalog       Catalog
	clock         clockwork.Clock
	client        remote.WriteClient
}

func newWriteLoop(dataDir string, destination *Destination, hcatalog Catalog, clock clockwork.Clock) *writeLoop {
	return &writeLoop{
		dataDir:     dataDir,
		destination: destination,
		catalog:     hcatalog,
		clock:       clock,
	}
}

// run sending data via RemoteWriter.
//
//revive:disable-next-line:cyclomatic // but readable
//revive:disable-next-line:function-length // long but readable
//revive:disable-next-line:cognitive-complexity // long but readable
func (wl *writeLoop) run(ctx context.Context) {
	var delay time.Duration
	var err error
	var i *Iterator
	var nextI *Iterator

	rw := &readyProtobufWriter{}

	dcfg := wl.destination.Config()
	wl.destination.metrics.maxNumShards.Set(float64(dcfg.QueueConfig.MaxShards))
	wl.destination.metrics.minNumShards.Set(float64(dcfg.QueueConfig.MinShards))

	defer func() {
		if i != nil {
			_ = i.Close()
		}
		if nextI != nil {
			_ = nextI.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wl.clock.After(delay):
			delay = 0
		}

		if i == nil {
			if nextI != nil {
				i = nextI
				nextI = nil
			} else {
				i, err = wl.nextIterator(ctx, rw)
				if err != nil {
					logger.Errorf("get current next iterator: %v", err)
					delay = defaultDelay
					continue
				}
			}
		}

		if wl.client == nil {
			wl.client, err = createClient(wl.destination.Config())
			if err != nil {
				logger.Errorf("create client: %v", err)
				delay = defaultDelay
				continue
			}

			rw.SetProtobufWriter(newProtobufWriter(wl.client))
		}

		if err = wl.write(ctx, i); err != nil {
			logger.Errorf("iterator write: %v", err)
			delay = defaultDelay
			continue
		}

		if nextI == nil {
			nextI, err = wl.nextIterator(ctx, rw)
			if err != nil {
				logger.Errorf("get next iterator: %v", err)
				delay = defaultDelay
				continue
			}
		}

		if err = i.Close(); err != nil {
			logger.Errorf("close iterator: %v", err)
			delay = defaultDelay
			continue
		}

		i = nil
	}
}

// createClient creates a new [remote.WriteClient].
//
//nolint:gocritic // hugeParam // this is a constructor for new client
func createClient(config DestinationConfig) (client remote.WriteClient, err error) {
	clientConfig := remote.ClientConfig{
		URL:              config.URL,
		Timeout:          config.RemoteTimeout,
		HTTPClientConfig: config.HTTPClientConfig,
		SigV4Config:      config.SigV4Config,
		AzureADConfig:    config.AzureADConfig,
		Headers:          config.Headers,
		RetryOnRateLimit: true,
	}

	client, err = remote.NewWriteClient(config.Name, &clientConfig)
	if err != nil {
		return nil, fmt.Errorf("falied to create client: %w", err)
	}

	return client, nil
}

// write writes data from iterator to the remote write storage.
func (*writeLoop) write(ctx context.Context, iterator *Iterator) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := iterator.Next(ctx)
			if err != nil {
				if errors.Is(err, ErrEndOfBlock) {
					return nil
				}
				logger.Errorf("iteration failed: %v", err)
			}
		}
	}
}

// nextIterator returns next iterator.
//
//revive:disable-next-line:cyclomatic // this is a constructor for new iterator
//revive:disable-next-line:function-length // this is a constructor for new iterator
func (wl *writeLoop) nextIterator(ctx context.Context, protobufWriter ProtobufWriter) (*Iterator, error) {
	var nextHeadRecord *catalog.Record
	var err error
	var cleanStart bool
	dcfg := wl.destination.Config()
	if wl.currentHeadID != nil {
		nextHeadRecord, err = nextHead(ctx, wl.dataDir, wl.catalog, *wl.currentHeadID)
	} else {
		var headFound bool
		nextHeadRecord, headFound, err = scanForNextHead(ctx, wl.dataDir, wl.catalog, dcfg.Name)
		cleanStart = !headFound
	}
	if err != nil {
		return nil, fmt.Errorf("find next head: %w", err)
	}
	headDir := filepath.Join(wl.dataDir, nextHeadRecord.Dir())
	crw, err := NewCursorReadWriter(
		filepath.Join(headDir, fmt.Sprintf("%s.cursor", dcfg.Name)),
		nextHeadRecord.NumberOfShards(),
	)
	if err != nil {
		return nil, fmt.Errorf("create cursor: %w", err)
	}

	crc32, err := dcfg.CRC32()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("calculate crc32: %w", err), crw.Close())
	}

	var discardCache bool
	if crw.GetConfigCRC32() != crc32 {
		if err = crw.SetConfigCRC32(crc32); err != nil {
			return nil, errors.Join(fmt.Errorf("write crc32: %w", err), crw.Close())
		}
		discardCache = true
	}

	ds, err := newDataSource(
		headDir,
		nextHeadRecord.NumberOfShards(),
		dcfg,
		discardCache,
		newSegmentReadyChecker(nextHeadRecord),
		wl.makeCorruptMarker(),
		nextHeadRecord,
		wl.destination.metrics.unexpectedEOFCount,
		wl.destination.metrics.segmentSizeInBytes,
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create data source: %w", err), crw.Close())
	}

	headID := nextHeadRecord.ID()
	ds.ID = headID

	var targetSegmentID uint32
	if cleanStart {
		if nextHeadRecord.LastAppendedSegmentID() != nil {
			targetSegmentID = *nextHeadRecord.LastAppendedSegmentID()
		} else {
			targetSegmentID = crw.GetTargetSegmentID()
		}
	} else {
		targetSegmentID = crw.GetTargetSegmentID()
	}

	i, err := newIterator(
		wl.clock,
		dcfg.QueueConfig,
		ds,
		crw,
		targetSegmentID,
		dcfg.ReadTimeout,
		protobufWriter,
		wl.destination.metrics,
	)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create data source: %w", err), crw.Close(), ds.Close())
	}

	wl.currentHeadID = &headID

	return i, nil
}

// CorruptMarkerFn func for mark head as corrupted by ID.
type CorruptMarkerFn func(headID string) error

// MarkCorrupted mark head as corrupted by ID.
func (fn CorruptMarkerFn) MarkCorrupted(headID string) error {
	return fn(headID)
}

// makeCorruptMarker set marker on head is corrupted.
func (wl *writeLoop) makeCorruptMarker() CorruptMarker {
	return CorruptMarkerFn(func(headID string) error {
		_, err := wl.catalog.SetCorrupted(headID)
		return err
	})
}

// nextHead returns next head record from catalog.
//
//nolint:gocritic // hugeParam // this is a extractor
//revive:disable-next-line:cyclomatic // this is a extractor
func nextHead(ctx context.Context, dataDir string, headCatalog Catalog, headID string) (*catalog.Record, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	headRecords := headCatalog.List(
		nil,
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)

	if len(headRecords) == 0 {
		return nil, fmt.Errorf("nextHead: no new heads: empty head records")
	}

	currentHeadFound := false
	for _, headRecord := range headRecords {
		if headRecord.ID() == headID {
			currentHeadFound = true
			continue
		}

		if !currentHeadFound {
			continue
		}

		if err := validateHead(filepath.Join(dataDir, headRecord.Dir())); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}

			switch headRecord.Status() {
			case catalog.StatusNew, catalog.StatusActive:
				return nil, fmt.Errorf("validate active head: %w", err)
			default:
				continue
			}
		}

		return headRecord, nil
	}

	// unknown head id, selecting last head
	if !currentHeadFound {
		return headRecords[len(headRecords)-1], nil
	}

	return nil, fmt.Errorf("nextHead: no new heads: appropriate head not found")
}

// validateHead validates head directory.
func validateHead(headDir string) error {
	dir, err := os.Open(headDir) // #nosec G304 // it's meant to be that way
	if err != nil {
		return err
	}

	return dir.Close()
}

// scanForNextHead scans catalog for next head record.
func scanForNextHead(
	ctx context.Context,
	dataDir string,
	headCatalog Catalog,
	destinationName string,
) (*catalog.Record, bool, error) {
	if err := contextErr(ctx); err != nil {
		return nil, false, err
	}

	headRecords := headCatalog.List(
		nil,
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() > rhs.CreatedAt()
		},
	)

	if len(headRecords) == 0 {
		return nil, false, fmt.Errorf("scanForNextHead: no new heads: empty head records")
	}

	for _, headRecord := range headRecords {
		headFound, err := scanHeadForDestination(filepath.Join(dataDir, headRecord.Dir()), destinationName)
		if err != nil {
			if !headRecord.Corrupted() {
				logger.Errorf("head %s is corrupted: %v", headRecord.ID(), err)
				if _, corruptErr := headCatalog.SetCorrupted(headRecord.ID()); corruptErr != nil {
					logger.Errorf("set corrupted state: %v", corruptErr)
				}
			}

			continue
		}
		if headFound {
			return headRecord, true, nil
		}
	}

	// track of the previous destination not found, selecting last head
	return headRecords[0], false, nil
}

// scanHeadForDestination scans head directory for [Destination].
func scanHeadForDestination(dirPath, destinationName string) (bool, error) {
	dir, err := os.Open(dirPath) // #nosec G304 // it's meant to be that way
	if err != nil {
		return false, fmt.Errorf("open head dir: %w", err)
	}
	defer func() { _ = dir.Close() }()

	fileNames, err := dir.Readdirnames(-1)
	if err != nil {
		return false, fmt.Errorf("read dir names: %w", err)
	}

	for _, fileName := range fileNames {
		if fileName == fmt.Sprintf("%s.cursor", destinationName) {
			return true, nil
		}
	}

	return false, nil
}

// contextErr returns error if context is done.
func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// readyProtobufWriter is a writer for ready protobuf.
type readyProtobufWriter struct {
	protobufWriter ProtobufWriter
}

// SetProtobufWriter sets protobuf writer.
func (rpw *readyProtobufWriter) SetProtobufWriter(protobufWriter ProtobufWriter) {
	rpw.protobufWriter = protobufWriter
}

// Write writes protobuf to the remote write storage.
func (rpw *readyProtobufWriter) Write(ctx context.Context, protobuf *cppbridge.SnappyProtobufEncodedData) error {
	return rpw.protobufWriter.Write(ctx, protobuf)
}
