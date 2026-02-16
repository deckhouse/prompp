package catalog

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

//
// Status
//

// Status of [Head] for record in catalog.
type Status uint8

const (
	// StatusNew status for new [Head].
	StatusNew Status = iota
	// StatusRotated status for rotated [Head].
	StatusRotated
	// StatusCorrupted status for corrupted [Head]. Deprecated.
	StatusCorrupted
	// StatusPersisted status for persisted [Head].
	StatusPersisted
	// StatusActive status for active [Head].
	StatusActive
)

// defaultSegments default number of segments in [WAL].
const defaultSegments = 3600 * 2 / 5

//
// Record
//

// Record information about the [Head] in the catalog.
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
	// marking up through segment IDs by shards
	nextSegmentID uint32
	// TODO clear segmentsByShard
	// (c *Catalog) Delete(id string) - delete record from memory(on catalog gc)
	// (c *Catalog) SetCorrupted(id string) - clear segmentsByShard ??? catalog gc skip Delete
	// reference GC if record.Corrupted() {
	segmentsByShard []uint16
	segmentsLock    *sync.RWMutex
}

// NewEmptyRecord init new empty [Record].
func NewEmptyRecord() *Record {
	return &Record{
		nextSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegments),
		segmentsLock:    &sync.RWMutex{},
	}
}

// NewRecordWithData init new [Record] with parameters.
func NewRecordWithData(
	id uuid.UUID,
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
		// marking up through segment IDs by shards
		nextSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegments),
		segmentsLock:    &sync.RWMutex{},
	}
}

// NewRecordWithDataV3 init new [Record] version 3 with parameters.
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
		// marking up through segment IDs by shards
		nextSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegments),
		segmentsLock:    &sync.RWMutex{},
	}
}

// Acquire increase reference count to [Head]. Returns func decrease reference count.
func (r *Record) Acquire() func() {
	atomic.AddInt64(&r.referenceCount, 1)
	var onceRelease sync.Once
	return func() {
		onceRelease.Do(func() {
			atomic.AddInt64(&r.referenceCount, -1)
		})
	}
}

// Corrupted returns true if [Head] is corrupted.
func (r *Record) Corrupted() bool {
	return r.corrupted
}

// CreatedAt returns the timestamp when the [Record]([Head]) was created.
func (r *Record) CreatedAt() int64 {
	return r.createdAt
}

// DeletedAt returns the timestamp when the [Record]([Head]) was deleted.
func (r *Record) DeletedAt() int64 {
	return r.deletedAt
}

// Dir returns dir of [Head].
func (r *Record) Dir() string {
	return r.id.String()
}

// GetShardBySegmentID returns the ID of the shard where the through segment ID is located.
func (r *Record) GetShardBySegmentID(sid uint32) uint16 {
	r.segmentsLock.RLock()
	defer r.segmentsLock.RUnlock()

	if len(r.segmentsByShard) > int(sid) {
		return r.segmentsByShard[sid] - 1
	}

	return math.MaxUint16
}

// ID returns id of [Head].
func (r *Record) ID() string {
	return r.id.String()
}

// LastAppendedSegmentID returns last appended segment id if exist, else nil.
func (r *Record) LastAppendedSegmentID() *uint32 {
	return r.lastAppendedSegmentID.RawValue()
}

// Maxt returns max timestamp in [Head].
func (r *Record) Maxt() int64 {
	return r.maxt
}

// Mint returns min timestamp in [Head].
func (r *Record) Mint() int64 {
	return r.mint
}

// NextSegmentID returns the next through ID for the segment.
func (r *Record) NextSegmentID() uint32 {
	return atomic.AddUint32(&r.nextSegmentID, 1)
}

// NumberOfSegments returns number of segments in [Head].
func (r *Record) NumberOfSegments() uint32 {
	return r.numberOfSegments
}

// NumberOfShards returns number of shards of [Head].
func (r *Record) NumberOfShards() uint16 {
	return r.numberOfShards
}

// ReferenceCount returns current of reference count.
func (r *Record) ReferenceCount() int64 {
	return atomic.LoadInt64(&r.referenceCount)
}

// SetLastAppendedSegmentID set last appended segment id.
func (r *Record) SetLastAppendedSegmentID(segmentID uint32) {
	r.lastAppendedSegmentID.Set(segmentID)
}

// SetNumberOfSegments number of segments in [Head].
func (r *Record) SetNumberOfSegments(numberOfSegments uint32) {
	r.numberOfSegments = numberOfSegments
}

// SetSegmentIDByShard sets the matching of through segment ID and shard.
func (r *Record) SetSegmentIDByShard(sid uint32, shardID uint16) {
	r.segmentsLock.Lock()
	defer r.segmentsLock.Unlock()

	// fmt.Println(" === SetSegmentIDByShard", sid, shardID)

	if len(r.segmentsByShard) > int(sid) {
		r.segmentsByShard[sid] = shardID + 1
		return
	}

	if cap(r.segmentsByShard) > int(sid) {
		r.segmentsByShard = r.segmentsByShard[:sid+1]
		r.segmentsByShard[sid] = shardID + 1
		return
	}

	r.segmentsByShard = append(
		r.segmentsByShard[:cap(r.segmentsByShard)],
		make([]uint16, int(sid)-cap(r.segmentsByShard)+1)...,
	)

	r.segmentsByShard[sid] = shardID + 1
}

// Status returns current status of [Head].
func (r *Record) Status() Status {
	return r.status
}

// UpdatedAt returns the timestamp when the [Record]([Head]) was updated.
func (r *Record) UpdatedAt() int64 {
	return r.updatedAt
}

// createRecordCopy create a copy of the [Record].
func createRecordCopy(r *Record) *Record {
	c := *r
	return &c
}

// applyRecordChanges apply changes to current [Record].
//
//go:norace
func applyRecordChanges(r, changed *Record) {
	r.createdAt = changed.createdAt
	r.updatedAt = changed.updatedAt
	r.deletedAt = changed.deletedAt
	r.corrupted = changed.corrupted
	r.status = changed.status
	r.numberOfShards = changed.numberOfShards
	r.mint = changed.mint
	r.maxt = changed.maxt
}

// LessByUpdateAt less [Record] by UpdateAt.
func LessByUpdateAt(lhs, rhs *Record) bool {
	return lhs.UpdatedAt() < rhs.UpdatedAt()
}
