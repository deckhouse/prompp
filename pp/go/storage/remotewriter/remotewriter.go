package remotewriter

import (
	"context"
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/ready"
)

//
// Catalog
//

// Catalog of current head records.
type Catalog interface {
	// List returns slice of records with filter and sort.
	List(
		filterFn func(record *catalog.Record) bool,
		sortLess func(lhs, rhs *catalog.Record) bool,
	) []*catalog.Record

	// SetCorrupted set corrupted flag for ID and returns [catalog.Record] if exist.
	SetCorrupted(id string) (*catalog.Record, error)
}

// RemoteWriter sent samples to the remote write storage.
type RemoteWriter struct {
	dataDir       string
	configQueue   chan []DestinationConfig
	catalog       Catalog
	clock         clockwork.Clock
	readyNotifier ready.Notifier
	registerer    prometheus.Registerer
}

// New init new [RemoteWriter].
func New(
	dataDir string,
	hcatalog Catalog,
	clock clockwork.Clock,
	readyNotifier ready.Notifier,
	registerer prometheus.Registerer,
) *RemoteWriter {
	return &RemoteWriter{
		dataDir:       dataDir,
		catalog:       hcatalog,
		clock:         clock,
		configQueue:   make(chan []DestinationConfig),
		readyNotifier: readyNotifier,
		registerer:    registerer,
	}
}

// ApplyConfig updates the state as the new config requires.
func (rw *RemoteWriter) ApplyConfig(configs ...DestinationConfig) (err error) {
	select {
	case rw.configQueue <- configs:
		return nil
	case <-time.After(time.Minute):
		return fmt.Errorf("failed to apply remote write configs, timeout")
	}
}

// Run sending data via RemoteWriter.
//
//revive:disable-next-line:cyclomatic but readable
//revive:disable-next-line:function-length long but readable
//revive:disable-next-line:cognitive-complexity function is not complicated.
func (rw *RemoteWriter) Run(ctx context.Context) error {
	writeLoopRunners := make(map[string]*writeLoopRunner)
	defer func() {
		for _, wlr := range writeLoopRunners {
			wlr.stop()
		}
	}()

	destinations := make(map[string]*Destination)

	for {
		select {
		case <-ctx.Done():
			return nil
		case configs := <-rw.configQueue:
			destinationConfigs := make(map[string]DestinationConfig)
			for i := range configs {
				destinationConfigs[configs[i].Name] = configs[i]
			}

			for _, destination := range destinations {
				name := destination.Config().Name
				if _, ok := destinationConfigs[name]; !ok {
					wlr := writeLoopRunners[name]
					wlr.stop()
					destination.UnregisterMetrics(rw.registerer)
					delete(destinations, name)
					delete(writeLoopRunners, name)
				}
			}

			for _, config := range configs { //nolint:gocritic // hugeParam // constructor
				destination, ok := destinations[config.Name]
				if !ok {
					destination = NewDestination(config)
					destination.RegisterMetrics(rw.registerer)
					wl := newWriteLoop(rw.dataDir, destination, rw.catalog, rw.clock)
					wlr := newWriteLoopRunner(wl)
					writeLoopRunners[config.Name] = wlr
					destinations[config.Name] = destination
					go wlr.run(ctx)
					continue
				}

				if config.EqualTo(destination.Config()) {
					continue
				}

				wlr := writeLoopRunners[config.Name]
				wlr.stop()
				destination.ResetConfig(config)
				wl := newWriteLoop(rw.dataDir, destination, rw.catalog, rw.clock)
				wlr = newWriteLoopRunner(wl)
				writeLoopRunners[config.Name] = wlr
				go wlr.run(ctx)
			}
			rw.readyNotifier.NotifyReady()
		}
	}
}

type writeLoopRunner struct {
	wl       *writeLoop
	stopc    chan struct{}
	stoppedc chan struct{}
}

func newWriteLoopRunner(wl *writeLoop) *writeLoopRunner {
	return &writeLoopRunner{
		wl:       wl,
		stopc:    make(chan struct{}),
		stoppedc: make(chan struct{}),
	}
}

func (r *writeLoopRunner) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-r.stopc
		cancel()
	}()
	r.wl.run(ctx)
	close(r.stoppedc)
}

func (r *writeLoopRunner) stop() {
	close(r.stopc)
	<-r.stoppedc
}
