package distributor

import (
	"context"
	"errors"
	"sync"

	"github.com/prometheus/prometheus/pp/go/relabeler"
)

type Distributor struct {
	lock              sync.Mutex
	destinationGroups relabeler.DestinationGroups
}

func NewDistributor(destinationGroups relabeler.DestinationGroups) *Distributor {
	return &Distributor{
		destinationGroups: destinationGroups,
	}
}

func (d *Distributor) DestinationGroups() relabeler.DestinationGroups {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.destinationGroups
}

func (d *Distributor) Rotate() error {
	return d.ParallelRange(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		destinationGroup.Rotate()
		return nil
	})
}

func (d *Distributor) SetDestinationGroups(destinationGroups relabeler.DestinationGroups) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.destinationGroups = destinationGroups
}

func (d *Distributor) Shutdown(ctx context.Context) error {
	return d.ParallelRange(func(_ int, destinationGroup *relabeler.DestinationGroup) error {
		return destinationGroup.Shutdown(ctx)
	})
}

func (d *Distributor) WriteMetrics(head relabeler.Head) {
	if d.Len() == 0 {
		return
	}

	_ = d.ParallelRange(func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error {
		destinationGroup.ObserveEncodersMemory()
		return nil
	})
}

func (d *Distributor) ParallelRange(fn func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) error) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	errs := make([]error, len(d.destinationGroups))
	wg := new(sync.WaitGroup)
	wg.Add(len(d.destinationGroups))
	for destinationGroupID, destinationGroup := range d.destinationGroups {
		go func(destinationGroupID int, destinationGroup *relabeler.DestinationGroup) {
			errs[destinationGroupID] = fn(destinationGroupID, destinationGroup)
			wg.Done()
		}(destinationGroupID, destinationGroup)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Len number of destinationGroups.
func (d *Distributor) Len() int {
	d.lock.Lock()
	length := len(d.destinationGroups)
	d.lock.Unlock()

	return length
}
