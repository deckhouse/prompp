package handler

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.uber.org/multierr"

	prom_config "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/value"
	ppmodel "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
	prometheustranslator "github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheus"
)

const (
	pbContentType   = "application/x-protobuf"
	jsonContentType = "application/json"
)

const (
	sumStr           = "_sum"
	countStr         = "_count"
	bucketStr        = "_bucket"
	leStr            = "le"
	pInfStr          = "+Inf"
	quantileStr      = "quantile"
	targetMetricName = "target_info"
)

var OTLPAlwaysCommit = true

// OTLPWriteHandler handler for otlp data via remote write.
type OTLPWriteHandler struct {
	logger   log.Logger
	receiver Receiver
}

func NewOTLPWriteHandler(logger log.Logger, receiver Receiver) *OTLPWriteHandler {
	return &OTLPWriteHandler{
		logger:   logger,
		receiver: receiver,
	}
}

// ServeHTTP implementation http.Handler.
func (h *OTLPWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := DecodeOTLPWriteRequest(r)
	if err != nil {
		level.Error(h.logger).Log("msg", "Error decoding remote write request", "err", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	converter := NewPPConverter(h.logger, req.Metrics().MetricCount())
	if err := converter.FromMetrics(req.Metrics()); err != nil {
		level.Warn(h.logger).Log("msg", "Error translating OTLP metrics to Prometheus write request", "err", err)
	}

	stats, err := h.receiver.AppendTimeSeries(
		r.Context(),
		converter.TimeSeries(),
		nil,
		prom_config.TransparentRelabeler,
		OTLPAlwaysCommit,
	)

	switch {
	case err == nil:
	case errors.Is(err, storage.ErrOutOfOrderSample),
		errors.Is(err, storage.ErrOutOfBounds),
		errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
		// Indicated an out of order sample is a bad request to prevent retries.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	default:
		level.Error(h.logger).Log("msg", "Error appending remote write", "err", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	level.Debug(h.logger).Log("msg", "append metrics", "stats", stats)

	w.WriteHeader(http.StatusOK)
}

// DecodeOTLPWriteRequest decode OTLP from http.Request.
func DecodeOTLPWriteRequest(r *http.Request) (pmetricotlp.ExportRequest, error) {
	contentType := r.Header.Get("Content-Type")
	var decoderFunc func(buf []byte) (pmetricotlp.ExportRequest, error)
	switch contentType {
	case pbContentType:
		decoderFunc = func(buf []byte) (pmetricotlp.ExportRequest, error) {
			req := pmetricotlp.NewExportRequest()
			return req, req.UnmarshalProto(buf)
		}

	case jsonContentType:
		decoderFunc = func(buf []byte) (pmetricotlp.ExportRequest, error) {
			req := pmetricotlp.NewExportRequest()
			return req, req.UnmarshalJSON(buf)
		}

	default:
		return pmetricotlp.NewExportRequest(), fmt.Errorf(
			"unsupported content type: %s, supported: [%s, %s]",
			contentType,
			jsonContentType,
			pbContentType,
		)
	}

	reader := r.Body
	// Handle compression.
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return pmetricotlp.NewExportRequest(), err
		}
		reader = gr

	case "":
		// No compression.

	default:
		return pmetricotlp.NewExportRequest(), fmt.Errorf(
			"unsupported compression: %s. Only \"gzip\" or no compression supported",
			r.Header.Get("Content-Encoding"),
		)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		r.Body.Close()
		return pmetricotlp.NewExportRequest(), err
	}
	if err = r.Body.Close(); err != nil {
		return pmetricotlp.NewExportRequest(), err
	}
	otlpReq, err := decoderFunc(body)
	if err != nil {
		return pmetricotlp.NewExportRequest(), err
	}

	return otlpReq, nil
}

//
// timeSeriesData
//

// timeSeriesData implementation relabeler.TimeSeriesData.
type timeSeriesData struct {
	timeSeries []ppmodel.TimeSeries
}

// TimeSeries return slice ppmodel.TimeSeries.
func (d *timeSeriesData) TimeSeries() []ppmodel.TimeSeries {
	return d.timeSeries
}

// Destroy implementation relabeler.TimeSeriesData.
func (d *timeSeriesData) Destroy() {
	d.timeSeries = nil
}

//
// PPConverter
//

// PPConverter converts from OTel write format to PP remote write format.
type PPConverter struct {
	labels     map[uint64][]*ppmodel.LabelSet
	timeSeries []ppmodel.TimeSeries
	logger     log.Logger
}

// NewPPConverter init new *PPConverter.
func NewPPConverter(logger log.Logger, count int) *PPConverter {
	return &PPConverter{
		labels:     make(map[uint64][]*ppmodel.LabelSet, count),
		timeSeries: make([]ppmodel.TimeSeries, 0, count),
		logger:     logger,
	}
}

// FromMetrics converts pmetric.Metrics to Prometheus remote write format.
func (c *PPConverter) FromMetrics(md pmetric.Metrics) (errs error) {
	resourceMetricsSlice := md.ResourceMetrics()
	for i := 0; i < resourceMetricsSlice.Len(); i++ {
		resourceMetrics := resourceMetricsSlice.At(i)
		resource := resourceMetrics.Resource()
		scopeMetricsSlice := resourceMetrics.ScopeMetrics()
		// keep track of the most recent timestamp in the ResourceMetrics for
		// use with the "target" info metric
		var mostRecentTimestamp pcommon.Timestamp
		for j := 0; j < scopeMetricsSlice.Len(); j++ {
			metricSlice := scopeMetricsSlice.At(j).Metrics()

			// TODO: decide if instrumentation library information should be exported as labels
			for k := 0; k < metricSlice.Len(); k++ {
				metric := metricSlice.At(k)
				mostRecentTimestamp = max(mostRecentTimestamp, mostRecentTimestampInMetric(metric))

				if !isValidAggregationTemporality(metric) {
					errs = multierr.Append(
						errs,
						fmt.Errorf("invalid temporality and type combination for metric %q", metric.Name()),
					)
					continue
				}

				promName := prometheustranslator.BuildCompliantName(metric, "", true)

				// handle individual metrics based on type
				//exhaustive:enforce
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dataPoints := metric.Gauge().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					c.addGaugeNumberDataPoints(dataPoints, resource, promName)
				case pmetric.MetricTypeSum:
					dataPoints := metric.Sum().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					c.addSumNumberDataPoints(dataPoints, resource, promName)
				case pmetric.MetricTypeHistogram:
					dataPoints := metric.Histogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					c.addHistogramDataPoints(dataPoints, resource, promName)
				case pmetric.MetricTypeExponentialHistogram:
					dataPoints := metric.ExponentialHistogram().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					errs = multierr.Append(errs, c.addExponentialHistogramDataPoints(
						dataPoints,
						resource,
						promName,
					))
				case pmetric.MetricTypeSummary:
					dataPoints := metric.Summary().DataPoints()
					if dataPoints.Len() == 0 {
						errs = multierr.Append(errs, fmt.Errorf("empty data points. %s is dropped", metric.Name()))
						break
					}
					c.addSummaryDataPoints(dataPoints, resource, promName)
				default:
					errs = multierr.Append(errs, errors.New("unsupported metric type"))
				}
			}
		}
		c.addResourceTargetInfo(resource, mostRecentTimestamp)
	}

	return
}

// TimeSeries returns a slice of the ppmodel.TimeSeries that were converted from OTel format.
func (c *PPConverter) TimeSeries() relabeler.TimeSeriesData {
	slices.SortFunc(
		c.timeSeries,
		func(a, b ppmodel.TimeSeries) int {
			return strings.Compare(a.LabelSet.String(), b.LabelSet.String())
		},
	)
	return &timeSeriesData{
		timeSeries: c.timeSeries,
	}
}

// addGaugeNumberDataPoints add Gauge DataPoints.
func (c *PPConverter) addGaugeNumberDataPoints(
	dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource,
	name string,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		labels := c.createAttributes(
			resource,
			pt.Attributes(),
			nil,
			true,
			model.MetricNameLabel,
			name,
		)

		var v float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			v = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			v = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			v = math.Float64frombits(value.StaleNaN)
		}
		c.addTimeSeries(labels, v, convertTimeStamp(pt.Timestamp()))
	}
}

// addSumNumberDataPoints add Sum DataPoints.
func (c *PPConverter) addSumNumberDataPoints(
	dataPoints pmetric.NumberDataPointSlice,
	resource pcommon.Resource,
	name string,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		lbls := c.createAttributes(
			resource,
			pt.Attributes(),
			nil,
			true,
			model.MetricNameLabel,
			name,
		)

		var v float64
		switch pt.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			v = float64(pt.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			v = pt.DoubleValue()
		}
		if pt.Flags().NoRecordedValue() {
			v = math.Float64frombits(value.StaleNaN)
		}
		c.addTimeSeries(lbls, v, convertTimeStamp(pt.Timestamp()))
	}
}

// addHistogramDataPoints add Histogram DataPoints.
func (c *PPConverter) addHistogramDataPoints(
	dataPoints pmetric.HistogramDataPointSlice,
	resource pcommon.Resource,
	baseName string,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := c.createAttributes(resource, pt.Attributes(), nil, false)

		// If the sum is unset, it indicates the _sum metric point should be
		// omitted
		if pt.HasSum() {
			// treat sum as a sample in an individual TimeSeries
			sum := pt.Sum()
			if pt.Flags().NoRecordedValue() {
				sum = math.Float64frombits(value.StaleNaN)
			}

			c.addTimeSeries(createLabels(baseName+sumStr, baseLabels), sum, timestamp)
		}

		// treat count as a sample in an individual TimeSeries
		count := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			count = math.Float64frombits(value.StaleNaN)
		}

		c.addTimeSeries(createLabels(baseName+countStr, baseLabels), count, timestamp)

		// cumulative count for conversion to cumulative histogram
		var cumulativeCount uint64

		// process each bound, based on histograms proto definition, # of buckets = # of explicit bounds + 1
		for i := 0; i < pt.ExplicitBounds().Len() && i < pt.BucketCounts().Len(); i++ {
			bound := pt.ExplicitBounds().At(i)
			cumulativeCount += pt.BucketCounts().At(i)

			bucket := float64(cumulativeCount)
			if pt.Flags().NoRecordedValue() {
				bucket = math.Float64frombits(value.StaleNaN)
			}

			c.addTimeSeries(
				createLabels(baseName+bucketStr, baseLabels, leStr, strconv.FormatFloat(bound, 'f', -1, 64)),
				bucket,
				timestamp,
			)
		}

		// add le=+Inf bucket
		var infBucket float64
		if pt.Flags().NoRecordedValue() {
			infBucket = math.Float64frombits(value.StaleNaN)
		} else {
			infBucket = float64(pt.Count())
		}

		c.addTimeSeries(createLabels(baseName+bucketStr, baseLabels, leStr, pInfStr), infBucket, timestamp)
	}
}

// addExponentialHistogramDataPoints add ExponentialHistogram DataPoints. NotNot implemented.
func (c *PPConverter) addExponentialHistogramDataPoints(
	_ pmetric.ExponentialHistogramDataPointSlice,
	_ pcommon.Resource,
	_ string,
) error {
	// skip, TODO Histograms Exemplars
	return nil
}

// addSummaryDataPoints add Summary DataPoints.
func (c *PPConverter) addSummaryDataPoints(
	dataPoints pmetric.SummaryDataPointSlice,
	resource pcommon.Resource,
	baseName string,
) {
	for x := 0; x < dataPoints.Len(); x++ {
		pt := dataPoints.At(x)
		timestamp := convertTimeStamp(pt.Timestamp())
		baseLabels := c.createAttributes(resource, pt.Attributes(), nil, false)

		// treat sum as a sample in an individual TimeSeries
		sum := pt.Sum()
		if pt.Flags().NoRecordedValue() {
			sum = math.Float64frombits(value.StaleNaN)
		}

		// sum and count of the summary should append suffix to baseName
		c.addTimeSeries(createLabels(baseName+sumStr, baseLabels), sum, timestamp)

		// treat count as a sample in an individual TimeSeries
		count := float64(pt.Count())
		if pt.Flags().NoRecordedValue() {
			count = math.Float64frombits(value.StaleNaN)
		}

		c.addTimeSeries(createLabels(baseName+countStr, baseLabels), count, timestamp)

		// process each percentile/quantile
		for i := 0; i < pt.QuantileValues().Len(); i++ {
			qt := pt.QuantileValues().At(i)
			quantile := qt.Value()
			if pt.Flags().NoRecordedValue() {
				quantile = math.Float64frombits(value.StaleNaN)
			}

			c.addTimeSeries(
				createLabels(baseName, baseLabels, quantileStr, strconv.FormatFloat(qt.Quantile(), 'f', -1, 64)),
				quantile,
				timestamp,
			)
		}
	}
}

// addResourceTargetInfo converts the resource to the target info metric.
func (c *PPConverter) addResourceTargetInfo(
	resource pcommon.Resource,
	timestamp pcommon.Timestamp,
) {
	if timestamp == 0 {
		return
	}

	attributes := resource.Attributes()
	identifyingAttrs := []string{
		conventions.AttributeServiceNamespace,
		conventions.AttributeServiceName,
		conventions.AttributeServiceInstanceID,
	}
	nonIdentifyingAttrsCount := attributes.Len()
	for _, a := range identifyingAttrs {
		_, haveAttr := attributes.Get(a)
		if haveAttr {
			nonIdentifyingAttrsCount--
		}
	}
	if nonIdentifyingAttrsCount == 0 {
		// If we only have job + instance, then target_info isn't useful, so don't add it.
		return
	}

	labels := c.createAttributes(resource, attributes, identifyingAttrs, false, model.MetricNameLabel, targetMetricName)
	haveIdentifier := false

	labels.Range(func(name, _ string) bool {
		if name == model.JobLabel || name == model.InstanceLabel {
			haveIdentifier = true
			return false
		}

		return true
	})

	if !haveIdentifier {
		// We need at least one identifying label to generate target_info.
		return
	}

	c.addTimeSeries(labels, 1, convertTimeStamp(timestamp))
}

// addTimeSeries add TimeSeries.
func (c *PPConverter) addTimeSeries(lbls ppmodel.LabelSet, v float64, ts int64) {
	if lbls.Len() == 0 {
		// This shouldn't happen
		return
	}

	c.timeSeries = append(
		c.timeSeries,
		ppmodel.TimeSeries{
			LabelSet:  *c.getLabelSet(lbls),
			Timestamp: uint64(ts),
			Value:     v,
		},
	)
}

// getLabelSet check labels in cache and return.
func (c *PPConverter) getLabelSet(lbls ppmodel.LabelSet) *ppmodel.LabelSet {
	h := lbls.Hash()

	for _, ls := range c.labels[h] {
		if lbls.String() == ls.String() {
			return ls
		}
	}

	c.labels[h] = append(c.labels[h], &lbls)

	return &lbls
}

// createAttributes create labels attributes.
func (c *PPConverter) createAttributes(
	resource pcommon.Resource,
	attributes pcommon.Map,
	ignoreAttrs []string,
	logOnOverwrite bool,
	extras ...string,
) ppmodel.LabelSet {
	resourceAttrs := resource.Attributes()
	serviceName, haveServiceName := resourceAttrs.Get(conventions.AttributeServiceName)
	instance, haveInstanceID := resourceAttrs.Get(conventions.AttributeServiceInstanceID)

	// Calculate the maximum possible number of labels we could return so we can preallocate l
	maxLabelCount := attributes.Len() + len(extras)/2

	if haveServiceName {
		maxLabelCount++
	}

	if haveInstanceID {
		maxLabelCount++
	}

	// map ensures no duplicate label name
	l := make(map[string]string, maxLabelCount)

	// Ensure attributes are sorted by key for consistent merging of keys which
	// collide when sanitized.
	labels := make([]ppmodel.SimpleLabel, 0, maxLabelCount)

	attributes.Range(func(key string, value pcommon.Value) bool {
		if !slices.Contains(ignoreAttrs, key) {
			labels = append(labels, ppmodel.SimpleLabel{Name: key, Value: value.AsString()})
		}

		return true
	})
	sort.Stable(ByLabelName(labels))

	for _, label := range labels {
		finalKey := prometheustranslator.NormalizeLabel(label.Name)
		if existingValue, alreadyExists := l[finalKey]; alreadyExists {
			l[finalKey] = existingValue + ";" + label.Value
		} else {
			l[finalKey] = label.Value
		}
	}

	// Map service.name + service.namespace to job
	if haveServiceName {
		val := serviceName.AsString()
		if serviceNamespace, ok := resourceAttrs.Get(conventions.AttributeServiceNamespace); ok {
			val = fmt.Sprintf("%s/%s", serviceNamespace.AsString(), val)
		}
		l[model.JobLabel] = val
	}

	// Map service.instance.id to instance
	if haveInstanceID {
		l[model.InstanceLabel] = instance.AsString()
	}

	for i := 0; i < len(extras); i += 2 {
		if i+1 >= len(extras) {
			break
		}
		_, found := l[extras[i]]
		if found && logOnOverwrite {
			level.Info(c.logger).Log(
				"msg", "label "+extras[i]+" is overwritten. Check if Prometheus reserved labels are used.",
			)
		}
		// internal labels should be maintained
		name := extras[i]
		if !(len(name) > 4 && name[:2] == "__" && name[len(name)-2:] == "__") {
			name = prometheustranslator.NormalizeLabel(name)
		}
		l[name] = extras[i+1]
	}

	return ppmodel.LabelSetFromMap(l)
}

//
// help
//

// ByLabelName enables the usage of sort.Sort() with a slice of labels.
type ByLabelName []ppmodel.SimpleLabel

func (a ByLabelName) Len() int           { return len(a) }
func (a ByLabelName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByLabelName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// mostRecentTimestampInMetric returns the latest timestamp in a batch of metrics.
func mostRecentTimestampInMetric(metric pmetric.Metric) pcommon.Timestamp {
	var ts pcommon.Timestamp
	// handle individual metric based on type
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		dataPoints := metric.Gauge().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSum:
		dataPoints := metric.Sum().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeHistogram:
		dataPoints := metric.Histogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeExponentialHistogram:
		dataPoints := metric.ExponentialHistogram().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	case pmetric.MetricTypeSummary:
		dataPoints := metric.Summary().DataPoints()
		for x := 0; x < dataPoints.Len(); x++ {
			ts = max(ts, dataPoints.At(x).Timestamp())
		}
	}
	return ts
}

// isValidAggregationTemporality checks whether an OTel metric has a valid
// aggregation temporality for conversion to a Prometheus metric.
func isValidAggregationTemporality(metric pmetric.Metric) bool {
	//exhaustive:enforce
	switch metric.Type() {
	case pmetric.MetricTypeGauge, pmetric.MetricTypeSummary:
		return true
	case pmetric.MetricTypeSum:
		return metric.Sum().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	case pmetric.MetricTypeHistogram:
		return metric.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	case pmetric.MetricTypeExponentialHistogram:
		return metric.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityCumulative
	}
	return false
}

// convertTimeStamp converts OTLP timestamp in ns to timestamp in ms.
func convertTimeStamp(timestamp pcommon.Timestamp) int64 {
	return timestamp.AsTime().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

// createLabels create labelset from base.
func createLabels(name string, baseLabels ppmodel.LabelSet, extras ...string) ppmodel.LabelSet {
	extraLabelCount := len(extras) / 2
	builder := ppmodel.NewLabelSetSimpleBuilderSize(baseLabels.Len() + extraLabelCount + 1) // +1 for name

	baseLabels.Range(func(name, value string) bool {
		builder.Add(name, value)

		return true
	})

	n := len(extras)
	n -= n % 2
	for extrasIdx := 0; extrasIdx < n; extrasIdx += 2 {
		builder.Set(extras[extrasIdx], extras[extrasIdx+1])
	}

	builder.Set(model.MetricNameLabel, name)

	return builder.Build()
}
