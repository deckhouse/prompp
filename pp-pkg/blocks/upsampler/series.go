package upsampler

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Series wraps a storage.Series and passes through Labels(),
// while Iterator() returns an upsampling iterator.
type Series struct {
	base    storage.Series
	rangeMs int64
}

// Labels returns the labels of the underlying series.
func (s *Series) Labels() labels.Labels {
	return s.base.Labels()
}

// Iterator returns an iterator over samples, with interpolation enabled.
// If the passed iterator is already an *Iterator (reuse pattern), reset it;
// otherwise create a new one.
func (s *Series) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	upsampler, ok := it.(*Iterator)
	if !ok {
		upsampler = NewIterator(nil, s.rangeMs)
	}

	baseIt := s.base.Iterator(nil)
	upsampler.Reset(baseIt, s.rangeMs)

	return upsampler
}
