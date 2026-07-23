//go:build !stringlabels && !dedupelabels && !cpplabels

package labels

import (
	"strings"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// builderPool builders pool for reuse in SeriesSet.
var builderPool = sync.Pool{
	New: func() any {
		b := NewScratchBuilder(10)
		return &b
	},
}

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetSnapshot, id uint32) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	builder := builderPool.Get().(*ScratchBuilder)
	builder.Reset()

	_ = lss.RangeLabelSet(id, func(l cppbridge.Label) error {
		// copy string from cpp memory
		builder.Add(strings.Clone(l.Name), strings.Clone(l.Value))

		return nil
	})

	lbls := builder.Labels()
	builderPool.Put(builder)
	return lbls
}
