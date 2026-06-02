package cppbridge

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util"
)

// defaultUpdateInterval is the interval between updates of [maxGaugeMetric].
const defaultUpdateInterval = 5 * time.Minute

// lssSetPendingShrinkBoundaryDurationMax is the max value of
// [LabelSetStorage].SetPendingShrinkBoundary() cpp call duration.
var lssSetPendingShrinkBoundaryDurationMax = newMaxGaugeMetric()

// [LabelSetStorage].SetPendingShrinkBoundary() cpp call duration.
var _ = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewGaugeFunc(
	prometheus.GaugeOpts{
		Name: "prompp_cppbridge_unsafecall_lss_set_pending_shrink_boundary_nanoseconds",
		Help: "The time duration lss set pending shrink boundary cpp call.",
	},
	lssSetPendingShrinkBoundaryDurationMax.get,
)

type maxGaugeMetric struct {
	mtx           sync.Mutex
	lastTimestamp time.Time
	lastValue     float64
}

// newMaxGaugeMetric create new [maxGaugeMetric].
func newMaxGaugeMetric() *maxGaugeMetric {
	return &maxGaugeMetric{
		lastTimestamp: time.Time{},
		lastValue:     0,
	}
}

// get current value of [maxGaugeMetric].
func (m *maxGaugeMetric) get() float64 {
	m.mtx.Lock()
	v := m.lastValue
	m.mtx.Unlock()

	return v
}

// set value to [maxGaugeMetric] and update it if it is more than last value.
// if the value is less than or equal to the last value, do nothing.
// if the value is more than the last value, update the last value and timestamp.
func (m *maxGaugeMetric) set(value float64) {
	now := time.Now()
	m.mtx.Lock()

	if now.Sub(m.lastTimestamp) > defaultUpdateInterval {
		m.lastTimestamp = now
		m.lastValue = value
		m.mtx.Unlock()

		return
	}

	if value <= m.lastValue {
		m.mtx.Unlock()
		return
	}

	m.lastValue = value
	m.lastTimestamp = now
	m.mtx.Unlock()
}
