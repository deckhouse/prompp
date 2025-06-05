//go:build cpplabels

package labels

import (
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

// Labels is a sorted set of labels. Is implemented by a cpp lss.
type Labels struct {
	snapshot       *cppbridge.LabelSetSnapshot
	id             uint32
	length         uint16
	dropMetricName bool
}

// EmptyLabels returns n null Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{}
}

// NewLabelsCppWithLSS init LabelsCpp with LabelSetStorage and ls id.
func NewLabelsWithLSS(
	lss *cppbridge.LabelSetSnapshot,
	id uint32,
	length uint16,
) Labels {
	return Labels{
		snapshot: lss,
		id:       id,
		length:   length,
	}
}

// New returns a sorted Labels from the given labels.
// The caller has to guarantee that all label names are unique.
func New(ls ...Label) Labels {
	builder := NewScratchBuilder(len(ls))
	for _, l := range ls {
		builder.Add(l.Name, l.Value)
	}

	return builder.Labels()
}

// FromStrings creates new labels from pairs of strings.
func FromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 { //revive:disable-line:add-constant // not need const
		panic("invalid number of strings")
	}

	builder := NewScratchBuilder(len(ss) / 2)
	for i := 0; i < len(ss); i += 2 { //revive:disable-line:add-constant // not need const
		builder.Add(ss[i], ss[i+1])
	}

	return builder.Labels()
}

// Bytes returns ls as a byte slice.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (ls Labels) Bytes(buf []byte) []byte {
	buf = buf[:0]
	if ls.IsZero() || ls.Len() == 0 {
		return append(buf, labelSep)
	}

	return ls.snapshot.LabelSetBytes(ls.id, buf, ls.dropMetricName)
}

// BytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithLabels(buf []byte, names ...string) []byte {
	buf = buf[:0]
	if ls.IsZero() || len(names) == 0 || ls.Len() == 0 {
		return append(buf, labelSep)
	}

	return ls.snapshot.LabelSetBytesWithLabels(ls.id, buf, ls.dropMetricName, names)
}

// BytesWithoutLabels is just as Bytes(), but only for labels not matching names.
// 'names' have to be sorted in ascending order.
func (ls Labels) BytesWithoutLabels(buf []byte, names ...string) []byte {
	buf = buf[:0]
	if ls.IsZero() || ls.Len() == 0 {
		return append(buf, labelSep)
	}

	return ls.snapshot.LabelSetBytesWithoutLabels(ls.id, buf, ls.dropMetricName, names)
}

// Copy returns a copy of the labels.
func (ls Labels) Copy() Labels {
	// labelset is immutable
	return ls
}

// CopyFrom copy labels from b on top of whatever was in ls previously, reusing memory or expanding if needed.
func (ls *Labels) CopyFrom(b Labels) {
	// tsdb/index/index.go 535
	*ls = b
}

// DropMetricName returns Labels with "__name__" removed.
func (ls Labels) DropMetricName() Labels {
	if ls.IsZero() || ls.Len() == 0 {
		return ls
	}

	ls.dropMetricName = true
	ls.length = uint16(ls.snapshot.LabelSetLength(ls.id, ls.dropMetricName))

	return ls
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return "" // Prometheus does not store blank label names.
	}

	if ls.IsZero() || ls.Len() == 0 {
		return ""
	}

	if ls.dropMetricName && name == MetricName {
		return ""
	}

	return ls.snapshot.LabelSetGetValue(ls.id, name)
}

// Has returns true if the label with the given name is present.
func (ls Labels) Has(name string) bool {
	if name == "" { // Avoid crash in loop if someone asks for "".
		return false // Prometheus does not store blank label names.
	}

	if ls.IsZero() || ls.Len() == 0 {
		return false
	}

	if ls.dropMetricName && name == MetricName {
		return false
	}

	return ls.snapshot.LabelSetHasLabelName(ls.id, name)
}

// HasDuplicateLabelNames returns whether ls has duplicate label names.
// It assumes that the labelset is sorted.
func (ls Labels) HasDuplicateLabelNames() (string, bool) {
	if ls.IsZero() || ls.Len() == 0 {
		return "", false
	}

	return ls.snapshot.LabelSetHasDuplicateLabelNames(ls.id, ls.dropMetricName)
}

// Hash returns a hash value for the label set.
// Note: the result is not guaranteed to be consistent across different runs of Prometheus.
func (ls Labels) Hash() uint64 {
	if ls.IsZero() || ls.Len() == 0 {
		return 0
	}

	return ls.snapshot.LabelSetHash(ls.id, ls.dropMetricName)
}

// HashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashForLabels(b []byte, names ...string) (uint64, []byte) {
	if ls.IsZero() || ls.Len() == 0 {
		return 0, b[:0]
	}

	return ls.snapshot.LabelSetHashForLabels(ls.id, names, ls.dropMetricName), b
}

// HashWithoutLabels returns a hash value for all labels except those matching
// the provided names. 'names' have to be sorted in ascending order.
func (ls Labels) HashWithoutLabels(b []byte, names ...string) (uint64, []byte) {
	if ls.IsZero() {
		return 0, b[:0]
	}

	return ls.snapshot.LabelSetHashWithoutLabels(ls.id, names), b
}

// InternStrings calls intern on every string value inside ls, replacing them with what it returns.
func (*Labels) InternStrings(func(string) string) {
	// remove these calls as there is nothing to do.
}

// ID return id labelset.
func (ls Labels) ID() uint32 {
	return ls.id
}

// IsEmpty returns true if ls represents an empty set of labels.
func (ls Labels) IsEmpty() bool {
	return ls.Len() == 0
}

// IsZero returns true if ls lss referece is nil.
// Implements yaml.IsZeroer - if we don't have this then 'omitempty' fields are always omitted.
func (ls Labels) IsZero() bool {
	if ls.snapshot != nil {
		if ls.length == 0 {
			ls.length = uint16(ls.snapshot.LabelSetLength(ls.id, ls.dropMetricName))
		}

		return ls.length == 0
	}

	return ls.snapshot == nil
}

// Len returns the number of labels.
func (ls Labels) Len() int {
	if ls.IsZero() {
		return 0
	}

	if ls.length == 0 {
		ls.length = uint16(ls.snapshot.LabelSetLength(ls.id, ls.dropMetricName))
	}

	return int(ls.length)
}

// MatchLabels returns a subset of Labels that matches/does not match with the provided label names based on the
// 'on' boolean. If on is set to true, it returns the subset of labels that match with the provided label names and
// its inverse when 'on' is set to false.
//
//revive:disable-next-line:flag-parameter implementation
func (ls Labels) MatchLabels(on bool, names ...string) Labels {
	if ls.IsZero() {
		return ls
	}

	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	builder := NewScratchBuilder(ls.Len())
	_ = ls.snapshot.RangeLabelSet(ls.id, ls.dropMetricName, func(l cppbridge.Label) error {
		if _, ok := nameSet[l.Name]; on == ok && (on || l.Name != MetricName) {
			builder.Add(l.Name, l.Value)
		}

		return nil
	})

	return builder.Labels()
}

// Range calls f on each label.
func (ls Labels) Range(f func(l Label)) {
	if ls.IsZero() {
		return
	}

	_ = ls.snapshot.RangeLabelSet(ls.id, ls.dropMetricName, func(l cppbridge.Label) error {
		f(Label(l))

		return nil
	})
}

// ReleaseStrings calls release on every string value inside ls.
func (Labels) ReleaseStrings(_ func(string)) {
	// remove these calls as there is nothing to do.
}

// Validate calls f on each label. If f returns a non-nil error, then it returns that error canceling the iteration.
func (ls Labels) Validate(f func(l Label) error) error {
	if ls.IsZero() {
		return nil
	}

	return ls.snapshot.RangeLabelSet(ls.id, ls.dropMetricName, func(l cppbridge.Label) error {
		return f(Label(l))
	})
}

// WithoutEmpty returns the labelset without empty labels.
// May return the same labelset.
func (ls Labels) WithoutEmpty() Labels {
	return ls
}

//
// ScratchBuilder
//

// ScratchBuilder allows efficient construction of a Labels from scratch.
type ScratchBuilder struct {
	builder model.LabelSetSimpleBuilder
}

// NewScratchBuilder creates a ScratchBuilder initialized for Labels with n entries.
func NewScratchBuilder(n int) ScratchBuilder {
	return ScratchBuilder{builder: *model.NewLabelSetSimpleBuilderSize(n)}
}

// NewScratchBuilderWithSymbolTable creates a ScratchBuilder, for api parity with dedupelabels.
func NewScratchBuilderWithSymbolTable(_ *SymbolTable, n int) ScratchBuilder {
	return NewScratchBuilder(n)
}

// Add a name/value pair.
// Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b *ScratchBuilder) Add(name, value string) {
	b.builder.Add(name, value)
}

// Assign is for when you already have a Labels which you want this ScratchBuilder to return.
func (b *ScratchBuilder) Assign(ls Labels) {
	b.builder.Reset()
	ls.Range(func(l Label) {
		b.builder.Add(l.Name, l.Value)
	})
}

// Labels returns the name/value pairs added as a Labels object. Calling Add() after Labels() has no effect.
func (b *ScratchBuilder) Labels() Labels {
	if b.builder.Len() == 0 {
		return EmptyLabels()
	}

	// isvalid
	return Storage.FindOrEmplaceLabelSet(b.builder.Build())
}

// Overwrite write the newly-built Labels out to ls.
func (b *ScratchBuilder) Overwrite(inls *Labels) {
	inls.CopyFrom(Storage.FindOrEmplaceLabelSet(b.builder.Build()))
}

// Reset clear builder container.
func (b *ScratchBuilder) Reset() {
	b.builder.Reset()
}

// SetSymbolTable implementation.
func (*ScratchBuilder) SetSymbolTable(*SymbolTable) {
	// no-op
}

// Sort the labels added so far by name.
func (b *ScratchBuilder) Sort() {
	b.builder.Sort()
}

// UnsafeAddBytes add a name/value pair, using []byte instead of string.
// The '-tags stringlabels' version of this function is unsafe, hence the name.
// This version is safe - it copies the strings immediately - but we keep the same name so everything compiles.
func (b *ScratchBuilder) UnsafeAddBytes(name, value []byte) {
	b.Add(
		unsafe.String(unsafe.SliceData(name), len(name)),   // #nosec G103 // it's meant to be that way
		unsafe.String(unsafe.SliceData(value), len(value)), // #nosec G103 // it's meant to be that way
	)
}

//
// Builder
//

// Builder allows modifying Labels.
type Builder struct {
	base Labels
	del  []string
	add  []Label
}

// NewBuilderWithSymbolTable creates a Builder, for api parity with dedupelabels.
func NewBuilderWithSymbolTable(*SymbolTable) *Builder {
	return NewBuilder(EmptyLabels())
}

// Reset clears all current state for the builder.
func (b *Builder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	b.base.Range(func(l Label) {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	})
}

// Labels returns the labels from the builder.
// If no modifications were made, the original labels are returned.
func (b *Builder) Labels() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	sbuilder := NewScratchBuilder(max(b.base.Len()+len(b.add)-len(b.del), 1))

	b.base.Range(func(l Label) {
		if slices.Contains(b.del, l.Name) || contains(b.add, l.Name) {
			return
		}

		sbuilder.Add(l.Name, l.Value)
	})

	for _, l := range b.add {
		sbuilder.Add(l.Name, l.Value)
	}

	return sbuilder.Labels()
}

//
// SymbolTable
//

// SymbolTable is no-op, just for api parity with dedupelabels.
type SymbolTable struct{}

// NewSymbolTable init SymbolTable.
func NewSymbolTable() *SymbolTable { return nil }

// Len implementation.
func (t *SymbolTable) Len() int { return 0 }

//
// help func
//

// Equal returns whether the two label sets are equal.
func Equal(a, b Labels) bool {
	if a.IsEmpty() && b.IsEmpty() {
		return true
	}

	if a.Len() != b.Len() {
		return false
	}

	return cppbridge.EqualLabelSets(
		a.snapshot, b.snapshot,
		a.id, b.id,
		a.dropMetricName, b.dropMetricName,
	)
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func Compare(a, b Labels) int {
	if a.IsEmpty() && b.IsEmpty() {
		return 0
	}

	return cppbridge.CompareLabelSets(
		a.snapshot, b.snapshot,
		a.id, b.id,
		a.dropMetricName, b.dropMetricName,
	)
}

//
// working
//

type Receiver interface {
	Find(mls model.LabelSet) Labels
}

var Storage = newStorage()

type storage struct {
	workingLSS      *cppbridge.LabelSetStorage
	workingSnapshot *cppbridge.LabelSetSnapshot
	receiver        atomic.Pointer[Receiver]
	mx              sync.Mutex

	lssMaxID    atomic.Uint32
	memoryInUse prometheus.Gauge
	maxID       prometheus.Gauge
}

// newStorage init new storage.
func newStorage() *storage {
	s := &storage{
		// workingLSS: cppbridge.NewLssStorage(),
		workingLSS: cppbridge.NewQueryableLssStorage(),
		memoryInUse: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_labels_working_lss_memory_bytes",
				Help: "Current value memory working lss in use in bytes.",
			},
		),
		maxID: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_labels_working_max_id",
				Help: "Current number max lss id.",
			},
		),
	}
	s.workingSnapshot = s.workingLSS.CreateLabelSetSnapshot()

	go s.writeMetrics()

	return s
}

// SetReceiver store Receiver.
func (s *storage) SetReceiver(receiver Receiver) {
	s.receiver.Store(&receiver)
}

// FindOrEmplaceLabelSet find ls in current lsses or store to worjing LSS and return Labels.
func (s *storage) FindOrEmplaceLabelSet(mls model.LabelSet) Labels {
	if receiver := s.receiver.Load(); receiver != nil {
		if ls := (*receiver).Find(mls); !ls.IsEmpty() {
			return ls
		}
	}

	s.mx.Lock()
	snapshot, lsID := s.workingLSS.FindOrEmplaceLabelSet(mls)
	if snapshot != nil {
		s.workingSnapshot = snapshot
	}
	s.mx.Unlock()

	s.lssMaxID.Store(max(lsID, s.lssMaxID.Load()))

	return NewLabelsWithLSS(s.workingSnapshot, lsID, uint16(mls.Len()))
}

// writeMetrics write metrics for working lss.
func (s *storage) writeMetrics() {
	ticker := time.NewTicker(30 * time.Second)

	for {
		s.mx.Lock()
		am := s.workingLSS.AllocatedMemory()
		s.mx.Unlock()

		s.memoryInUse.Set(float64(am))
		s.maxID.Set(float64(s.lssMaxID.Load()))

		<-ticker.C
	}
}
