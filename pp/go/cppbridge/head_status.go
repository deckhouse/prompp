package cppbridge

import (
	"runtime"
)

// HeadStatus statistics of heads.
type HeadStatus struct {
	TimeInterval struct {
		Min int64
		Max int64
	}
	LabelValueCountByLabelName []struct {
		Name  string
		Count uint32
	}
	SeriesCountByMetricName []struct {
		Name  string
		Count uint32
	}
	MemoryInBytesByLabelName []struct {
		Name string
		Size uint32
	}
	SeriesCountByLabelValuePair []struct {
		Name  string
		Value string
		Count uint32
	}
	NumSeries     uint32
	ChunkCount    uint32
	NumLabelPairs uint32
}

// NewHeadStatus init new HeadStatus.
func NewHeadStatus() *HeadStatus {
	hs := &HeadStatus{}
	runtime.SetFinalizer(hs, func(status *HeadStatus) {
		freeHeadStatus(status)
	})

	return hs
}

// FromLSS get head status from lss.
func (s *HeadStatus) FromLSS(lss *LabelSetStorage, limit int) {
	getHeadStatusLSS(lss.pointer, s, limit)
}

// FromDataStorage get head status from data storage.
func (s *HeadStatus) FromDataStorage(dataStorage *HeadDataStorage) {
	getHeadStatusDataStorage(dataStorage.dataStorage, s)
}
