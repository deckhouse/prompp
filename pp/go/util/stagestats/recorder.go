// Package stagestats accounts the time spent in the stages of a hot-path pipeline at minimal cost: monotonic clock
// readings, per-stage sum and count kept in striped atomic counters, exported as a pair of counter families.
package stagestats

import (
	"math/bits"
	"math/rand/v2"
	"runtime"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// maxStripes caps the number of stripes in a [Recorder].
	maxStripes = 64

	// stripePadding is the number of counters between stripes, it is a cache line,
	// so that stripes never share a cache line.
	stripePadding = 4
)

//
// Opts
//

// Opts are the options of a [Recorder].
type Opts struct {
	// Name is the prefix of the metric families: <Name>_duration_nanoseconds_total and <Name>_executions_total.
	Name string
	// Help is the description of the pipeline whose stages are accounted.
	Help string
	// ConstLabels are added to all metrics.
	ConstLabels prometheus.Labels
}

//
// counter
//

// counter accumulates the observations of one stage in one stripe.
type counter struct {
	sum   atomic.Uint64
	count atomic.Uint64
}

//
// Recorder
//

// Recorder accounts the duration and the number of executions of each stage of a pipeline
// and exports them as [prometheus.Collector]. A nil [Recorder] is valid and accounts nothing.
type Recorder struct {
	stages        []string
	counters      []counter
	stride        int
	stripeMask    uint32
	durationDesc  *prometheus.Desc
	executionDesc *prometheus.Desc
}

// NewRecorder init new [Recorder] for the given stage names and registers it in r, or returns the already
// registered [Recorder] with the same opts. A nil r returns an unregistered [Recorder].
func NewRecorder(r prometheus.Registerer, opts Opts, stages []string) *Recorder {
	return util.MustRegisterOrGet(r, newRecorder(opts, stages, defaultStripes()))
}

// newRecorder init new [Recorder] with the given number of stripes, rounded up to a power of two.
func newRecorder(opts Opts, stages []string, stripes int) *Recorder {
	stripes = 1 << bits.Len(uint(stripes-1))
	stride := len(stages) + stripePadding

	return &Recorder{
		stages:     stages,
		counters:   make([]counter, stripes*stride),
		stride:     stride,
		stripeMask: uint32(stripes - 1), // #nosec G115 // stripes <= maxStripes
		durationDesc: prometheus.NewDesc(
			opts.Name+"_duration_nanoseconds_total",
			opts.Help+" Total duration of the stage in nanoseconds.",
			[]string{"stage"},
			opts.ConstLabels,
		),
		executionDesc: prometheus.NewDesc(
			opts.Name+"_executions_total",
			opts.Help+" Total number of the stage executions.",
			[]string{"stage"},
			opts.ConstLabels,
		),
	}
}

// defaultStripes returns the default number of stripes: GOMAXPROCS capped by maxStripes.
func defaultStripes() int {
	return min(runtime.GOMAXPROCS(0), maxStripes)
}

// Collect implements [prometheus.Collector].
func (r *Recorder) Collect(ch chan<- prometheus.Metric) {
	for stage, name := range r.stages {
		var sum, count uint64
		for base := 0; base < len(r.counters); base += r.stride {
			c := &r.counters[base+stage]
			sum += c.sum.Load()
			count += c.count.Load()
		}

		ch <- prometheus.MustNewConstMetric(r.durationDesc, prometheus.CounterValue, float64(sum), name)
		ch <- prometheus.MustNewConstMetric(r.executionDesc, prometheus.CounterValue, float64(count), name)
	}
}

// Describe implements [prometheus.Collector].
func (r *Recorder) Describe(ch chan<- *prometheus.Desc) {
	ch <- r.durationDesc
	ch <- r.executionDesc
}

// RandomStripe returns a randomly selected [Stripe]. A nil [Recorder] returns a no-op [Stripe].
func (r *Recorder) RandomStripe() Stripe {
	if r == nil {
		return Stripe{}
	}

	return r.Stripe(rand.Uint32()) // #nosec G404 // it's only a stripe selection
}

// Start starts a [Lap] on a randomly selected [Stripe]. A nil [Recorder] returns a no-op [Lap].
func (r *Recorder) Start() Lap {
	if r == nil {
		return Lap{}
	}

	now := Now()

	return Lap{stripe: r.RandomStripe(), start: now, prev: now}
}

// Stripe returns the [Stripe] selected by the hint. A nil [Recorder] returns a no-op [Stripe].
func (r *Recorder) Stripe(hint uint32) Stripe {
	if r == nil {
		return Stripe{}
	}

	base := int(hint&r.stripeMask) * r.stride
	return Stripe{counters: r.counters[base : base+len(r.stages) : base+len(r.stages)]}
}
