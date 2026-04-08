package catalog

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

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

// defaultSegmentsCapacity is the minimum number of segments for one shard when
// segments are created only on the 5s flush timeout (2 hours / 5s).
const defaultSegmentsCapacity = int(2 * time.Hour / (5 * time.Second))

//
// SerializedRecord
//

// SerializedRecord is the serialized record for write/read to [Log].
type SerializedRecord struct {
	id                    uuid.UUID // uuid
	numberOfShards        uint16    // number of shards
	createdAt             int64     // time of record creation
	updatedAt             int64
	deletedAt             int64
	corrupted             bool
	lastAppendedSegmentID optional.Optional[uint32]
	status                Status // status
	numberOfSegments      uint32
	mint                  int64
	maxt                  int64
}

// createRecordCopy create a copy of the [Record].
func createSerializedRecordCopy(r *SerializedRecord) *SerializedRecord {
	c := *r
	return &c
}

//
// Record
//

// Record information about the [Head] in the catalog.
type Record struct {
	SerializedRecord
	// referenceCount is the reference count of the [Head]
	referenceCount int64
	// marking up through segment IDs by shards
	lastSegmentID   uint32
	segmentsByShard []uint16
	segmentsLock    *sync.RWMutex
}

// NewEmptyRecord init new empty [Record].
func NewEmptyRecord() *Record {
	return &Record{
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
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
		SerializedRecord: SerializedRecord{
			id:                    id,
			numberOfShards:        numberOfShards,
			createdAt:             createdAt,
			updatedAt:             updatedAt,
			deletedAt:             deletedAt,
			corrupted:             corrupted,
			status:                status,
			lastAppendedSegmentID: optional.WithRawValue(lastAppendedSegmentID),
		},
		referenceCount: referenceCount,
		// marking up through segment IDs by shards
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
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
		SerializedRecord: SerializedRecord{
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
		},
		// marking up through segment IDs by shards
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
		segmentsLock:    &sync.RWMutex{},
	}
}

// Acquire increase reference count to [Head]. Returns func decrease reference count.
func (r *Record) Acquire() func() {
	atomic.AddInt64(&r.referenceCount, 1)
	var onceRelease sync.Once
	return func() {
		onceRelease.Do(func() {
			if atomic.AddInt64(&r.referenceCount, -1) == 0 && r.status != StatusActive {
				r.ClearSegmentsByShard()
			}
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

// ClearSegmentsByShard remove the shard segment markup.
func (r *Record) ClearSegmentsByShard() {
	r.segmentsByShard = nil
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

// IsMissingSegmentsByShard returns true if there are missing segments by shard.
func (r *Record) IsMissingSegmentsByShard() bool {
	//revive:disable-next-line:add-constant // for length 1 not missing segments
	if len(r.segmentsByShard) < 2 {
		return false
	}

	//revive:disable-next-line:add-constant // start checking from 2 not missing segments
	for i := 2; i < len(r.segmentsByShard); i++ {
		if r.segmentsByShard[i] != 0 && r.segmentsByShard[i-1] == 0 {
			return true
		}
	}

	return false
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
	return atomic.AddUint32(&r.lastSegmentID, 1)
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

// SetLastSegmentID set last through ID for the segment, if sid more current.
func (r *Record) SetLastSegmentID(sid uint32) {
	if r.lastSegmentID != math.MaxUint32 && r.lastSegmentID >= sid {
		return
	}

	r.lastSegmentID = sid
}

// SetSegmentIDByShard sets the matching of through segment ID and shard.
func (r *Record) SetSegmentIDByShard(sid uint32, shardID uint16) {
	r.segmentsLock.Lock()
	defer r.segmentsLock.Unlock()

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

// applyRecordChanges apply changes to current [Record].
//
//go:norace
func applyRecordChanges(r *Record, changed *SerializedRecord) {
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
