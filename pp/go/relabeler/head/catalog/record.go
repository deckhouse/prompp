package catalog

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

type Status uint8

const (
	StatusNew Status = iota
	StatusRotated
	// Deprecated
	StatusCorrupted
	StatusPersisted
	StatusActive
)

type Record struct {
	id                    uuid.UUID // uuid
	numberOfShards        uint16    // number of shards
	createdAt             int64     // time of record creation
	updatedAt             int64
	deletedAt             int64
	corrupted             bool
	lastAppendedSegmentID optional.Optional[uint32]
	referenceCount        int64
	status                Status // status
	numberOfSegments      uint32
	mint                  int64
	maxt                  int64
}

func NewRecord() *Record {
	return &Record{}
}

func (r *Record) ID() string {
	return r.id.String()
}

func (r *Record) Dir() string {
	return r.id.String()
}

func (r *Record) NumberOfShards() uint16 {
	return r.numberOfShards
}

func (r *Record) CreatedAt() int64 {
	return r.createdAt
}

func (r *Record) UpdatedAt() int64 {
	return r.updatedAt
}

func (r *Record) DeletedAt() int64 {
	return r.deletedAt
}

func (r *Record) Corrupted() bool {
	return r.corrupted
}

func (r *Record) Status() Status {
	return r.status
}

func (r *Record) NumberOfSegments() uint32 {
	return r.numberOfSegments
}

func (r *Record) Mint() int64 {
	return r.mint
}

func (r *Record) Maxt() int64 {
	return r.maxt
}

func (r *Record) ReferenceCount() int64 {
	return atomic.LoadInt64(&r.referenceCount)
}

func (r *Record) Acquire() func() {
	atomic.AddInt64(&r.referenceCount, 1)
	var onceRelease sync.Once
	return func() {
		onceRelease.Do(func() {
			atomic.AddInt64(&r.referenceCount, -1)
		})
	}
}

//func (r *Record) LastAppendedSegmentID() *uint32 {
//	return r.lastAppendedSegmentID.RawValue()
//}
//
//func (r *Record) SetLastAppendedSegmentID(segmentID uint32) {
//	r.lastAppendedSegmentID.Set(segmentID)
//}

func (r *Record) SetNumberOfSegments(numberOfSegments uint32) {
	r.numberOfSegments = numberOfSegments
	if numberOfSegments > 0 {
		r.lastAppendedSegmentID.Set(numberOfSegments)
	} else {
		r.lastAppendedSegmentID = optional.Optional[uint32]{}
	}
}

func NewRecordWithData(id uuid.UUID,
	numberOfShards uint16,
	createdAt int64,
	updatedAt int64,
	deletedAt int64,
	corrupted bool,
	referenceCount int64,
	status Status,
	lastAppendedSegmentID *uint32,
) *Record {
	return &Record{
		id:                    id,
		numberOfShards:        numberOfShards,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
		deletedAt:             deletedAt,
		corrupted:             corrupted,
		referenceCount:        referenceCount,
		status:                status,
		lastAppendedSegmentID: optional.WithRawValue(lastAppendedSegmentID),
	}
}

func NewRecordWithDataV3(
	id uuid.UUID,
	numberOfShards uint16,
	createdAt int64,
	updatedAt int64,
	deletedAt int64,
	corrupted bool,
	status Status,
	numberOfSegments uint32,
	mint int64,
	maxt int64,
) *Record {
	return &Record{
		id:               id,
		numberOfShards:   numberOfShards,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
		deletedAt:        deletedAt,
		corrupted:        corrupted,
		status:           status,
		numberOfSegments: numberOfSegments,
		mint:             mint,
		maxt:             maxt,
	}
}

func createRecordCopy(r *Record) *Record {
	c := *r
	return &c
}

func applyRecordChanges(r *Record, changed *Record) {
	r.createdAt = changed.createdAt
	r.updatedAt = changed.updatedAt
	r.deletedAt = changed.deletedAt
	r.corrupted = changed.corrupted
	r.status = changed.status
	r.numberOfShards = changed.numberOfShards
	r.mint = changed.mint
	r.maxt = changed.maxt
}
