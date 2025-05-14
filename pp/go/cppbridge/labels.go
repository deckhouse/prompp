package cppbridge

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/mailru/easyjson/jwriter"
	"github.com/prometheus/common/model"
)

var seps = []byte{'\xff'}

// Label is a key/value pair of strings.
type Label struct {
	Name  string
	Value string
}

// Labels slice pairs of strings, used for data exchenge between Go and C++.
type Labels []Label

// EmptyLabels returns an empty Labels value, for convenience.
func EmptyLabels() Labels {
	return Labels{}
}

// FromMap returns new sorted Labels from the given map.
func FromMap(m map[string]string) Labels {
	ls := make([]Label, 0, len(m))
	for name, value := range m {
		ls = append(ls, Label{Name: name, Value: value})
	}

	slices.SortFunc(ls, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })

	return ls
}

// FromStrings creates new labels from pairs of strings.
func FromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 { //revive:disable:add-constant pairs
		panic("invalid number of strings")
	}

	ls := make([]Label, 0, len(ss)/2) //revive:disable:add-constant pairs
	for i := 0; i < len(ss); i += 2 { //revive:disable:add-constant pairs
		ls = append(ls, Label{Name: ss[i], Value: ss[i+1]})
	}

	slices.SortFunc(ls, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })

	return ls
}

// Hash returns a hash value for the label set.
// Note: the result is not guaranteed to be consistent across different runs of Prometheus.
func (ls Labels) Hash() uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024) //rrevive:disable:add-constant 1kb
	for i, v := range ls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) { //revive:disable:add-constant pairs
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

// IsEmpty returns true if ls represents an empty set of labels.
func (ls Labels) IsEmpty() bool {
	return len(ls) == 0
}

// IsValid checks if the metric name or label names are valid.
func (ls Labels) IsValid() bool {
	for _, l := range ls {
		if l.Name == model.MetricNameLabel && !model.IsValidMetricName(model.LabelValue(l.Value)) {
			return false
		}

		if !model.LabelName(l.Name).IsValid() || !model.LabelValue(l.Value).IsValid() {
			return false
		}
	}

	return true
}

// Len returns the number of labels.
func (ls Labels) Len() int {
	return len(ls)
}

// Map returns a string map of the labels.
func (ls Labels) Map() map[string]string {
	m := make(map[string]string, len(ls))

	for _, l := range ls {
		m[l.Name] = l.Value
	}

	return m
}

// MarshalJSON implements json.Marshaler.
func (ls Labels) MarshalJSON() ([]byte, error) {
	w := &jwriter.Writer{}
	w.RawByte('{')
	first := true
	for i := range ls {
		if !first {
			w.RawByte(',')
		}
		w.String(ls[i].Name)
		w.RawByte(':')
		w.String(ls[i].Value)
		first = false
	}
	w.RawByte('}')
	return w.BuildBytes()
}

// MarshalYAML implements yaml.Marshaler.
func (ls Labels) MarshalYAML() (any, error) {
	return ls.Map(), nil
}

// Range calls f on each label.
func (ls Labels) Range(f func(l Label)) {
	for _, l := range ls {
		f(l)
	}
}

// String serialize to string.
func (ls Labels) String() string {
	var bytea [1024]byte // On stack to avoid memory allocation while building the output.
	b := bytes.NewBuffer(bytea[:0])

	_ = b.WriteByte('{')

	for i, l := range ls {
		if i > 0 {
			_ = b.WriteByte(',')
			_ = b.WriteByte(' ')
		}
		_, _ = b.WriteString(l.Name)
		_ = b.WriteByte('=')
		_, _ = b.Write(strconv.AppendQuote(b.AvailableBuffer(), l.Value))
	}

	_ = b.WriteByte('}')

	return b.String()
}

// UnmarshalJSON implements json.Unmarshaler.
func (ls *Labels) UnmarshalJSON(b []byte) error {
	var m map[string]string

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (ls *Labels) UnmarshalYAML(unmarshal func(any) error) error {
	var m map[string]string

	if err := unmarshal(&m); err != nil {
		return err
	}

	*ls = FromMap(m)
	return nil
}

// Validate calls f on each label. If f returns a non-nil error, then it returns that error canceling the iteration.
func (ls Labels) Validate(f func(l Label) error) error {
	for _, l := range ls {
		if err := f(l); err != nil {
			return err
		}
	}
	return nil
}

//
// ScratchBuilder
//

// ScratchBuilder allows efficient construction of a Labels from scratch.
type ScratchBuilder struct {
	add    Labels
	sorted bool
}

// NewScratchBuilder creates a ScratchBuilder initialized for Labels with n entries.
func NewScratchBuilder(n int) ScratchBuilder {
	return ScratchBuilder{add: make([]Label, 0, n)}
}

// Add a name/value pair. Note if you Add the same name twice you will get a duplicate label, which is invalid.
func (b *ScratchBuilder) Add(name, value string) {
	b.add = append(b.add, Label{Name: name, Value: value})
	n := len(b.add)
	//revive:disable-next-line:add-constant not need to const
	b.sorted = b.sorted && (n > 1 && b.add[n-1].Name > b.add[n-2].Name)
}

// Labels return the name/value pairs added so far as a Labels object.
func (b *ScratchBuilder) Labels() Labels {
	if !b.sorted {
		b.Sort()
	}

	// Copy the slice, so the next use of ScratchBuilder doesn't overwrite.
	return slices.Clone(b.add)
}

// Reset clear builder container.
func (b *ScratchBuilder) Reset() {
	b.add = b.add[:0]
	b.sorted = false
}

// Sort the labels added so far by name.
func (b *ScratchBuilder) Sort() {
	if b.sorted {
		return
	}

	slices.SortFunc(b.add, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
	b.sorted = true
}

// Equal returns whether the two label sets are equal.
func Equal(ls, o Labels) bool {
	if len(ls) != len(o) {
		return false
	}

	for i, l := range ls {
		if l != o[i] {
			return false
		}
	}

	return true
}

// func LabelsToCppBridgeLabels(lbls labels.Labels) []Label {
// 	result := make([]Label, 0, lbls.Len())
// 	lbls.Range(func(l labels.Label) {
// 		result = append(result, Label{
// 			Name:  l.Name,
// 			Value: l.Value,
// 		})
// 	})
// 	return result
// }
