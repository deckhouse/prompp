package util

import "github.com/prometheus/client_golang/prometheus"

// UnconflictRegisterer is a prometheus.UnconflictRegisterer wrap to avoid duplicates errors.
type UnconflictRegisterer struct {
	prometheus.Registerer
}

// NewUnconflictRegisterer is the constructor.
func NewUnconflictRegisterer(r prometheus.Registerer) UnconflictRegisterer {
	return UnconflictRegisterer{r}
}

// NewCounter create new prometheus.Counter and register it in wrapped registerer.
func (cr UnconflictRegisterer) NewCounter(opts prometheus.CounterOpts) prometheus.Counter {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames := cr.extractConstLabelNames(constLabels, nil)
	c := prometheus.NewCounterVec(opts, labelNames)
	c = mustRegisterOrGet(cr.Registerer, c)
	return c.With(constLabels)
}

// NewCounterVec create new prometheus.CounterVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames = cr.extractConstLabelNames(constLabels, labelNames)
	c := prometheus.NewCounterVec(opts, labelNames)
	c = mustRegisterOrGet(cr.Registerer, c)
	return c.MustCurryWith(constLabels)
}

// NewGauge create new prometheus.Gauge and register it in wrapped registerer
func (cr UnconflictRegisterer) NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames := cr.extractConstLabelNames(constLabels, nil)
	c := prometheus.NewGaugeVec(opts, labelNames)
	c = mustRegisterOrGet(cr.Registerer, c)
	return c.With(constLabels)
}

// NewGaugeVec create new prometheus.GaugeVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames = cr.extractConstLabelNames(constLabels, labelNames)
	g := prometheus.NewGaugeVec(opts, labelNames)
	g = mustRegisterOrGet(cr.Registerer, g)
	return g.MustCurryWith(constLabels)
}

// NewHistogramVec create new prometheus.HistogramVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewHistogramVec(
	//nolint:gocritic // should be compatible with prometheus.NewHistogramVec
	opts prometheus.HistogramOpts, labelNames []string,
) *prometheus.HistogramVec {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames = cr.extractConstLabelNames(constLabels, labelNames)
	h := prometheus.NewHistogramVec(opts, labelNames)
	h = mustRegisterOrGet(cr.Registerer, h)
	return h.MustCurryWith(constLabels).(*prometheus.HistogramVec)
}

// NewHistogram create new prometheus.Histogram and register it in wrapped registerer
func (cr UnconflictRegisterer) NewHistogram(
	//nolint:gocritic // should be compatible with prometheus.NewHistogramVec
	opts prometheus.HistogramOpts,
) prometheus.Histogram {
	constLabels := opts.ConstLabels
	opts.ConstLabels = nil
	labelNames := cr.extractConstLabelNames(constLabels, nil)
	h := prometheus.NewHistogramVec(opts, labelNames)
	h = mustRegisterOrGet(cr.Registerer, h)
	return h.With(constLabels).(prometheus.Histogram)
}

func (UnconflictRegisterer) extractConstLabelNames(constLabels prometheus.Labels, labelNames []string) []string {
	if len(constLabels) == 0 {
		return labelNames
	}
	if len(labelNames) == 0 {
		labelNames = make([]string, 0, len(constLabels))
	}
	for name := range constLabels {
		labelNames = append(labelNames, name)
	}
	return labelNames
}

func mustRegisterOrGet[Collector prometheus.Collector](r prometheus.Registerer, c Collector) Collector {
	if r == nil {
		return c
	}
	err := r.Register(c)
	if err == nil {
		return c
	}
	if arErr, ok := err.(prometheus.AlreadyRegisteredError); ok {
		return arErr.ExistingCollector.(Collector)
	}
	panic(err)
}
