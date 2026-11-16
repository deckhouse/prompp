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
// fieldMask
//

// fieldMask is a set of [SerializedRecord] fields present in a record of the [Log].
type fieldMask uint16

const (
	fieldNumberOfShards fieldMask = 1 << iota
	fieldCreatedAt
	fieldUpdatedAt
	fieldDeletedAt
	fieldCorrupted
	fieldStatus
	fieldSegmentsCount
	fieldMinT
	fieldMaxT

	// fieldsAll is the set of all fields.
	fieldsAll = fieldNumberOfShards | fieldCreatedAt | fieldUpdatedAt | fieldDeletedAt | fieldCorrupted |
		fieldStatus | fieldSegmentsCount | fieldMinT | fieldMaxT
	// fieldsSnapshotV2 is the set of fields of a full snapshot of the log-file version 1 and 2,
	// which do not contain the time bounds.
	fieldsSnapshotV2 = fieldsAll &^ (fieldMinT | fieldMaxT)
)

// has returns true if all the fields of other are present in the mask.
func (m fieldMask) has(other fieldMask) bool {
	return m&other == other
}

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
	// fields present in the record of the [Log], not used by the in-memory records.
	fields fieldMask
}

// HasTimeBounds returns true if the time bounds of the [Head] data are known.
func (sr *SerializedRecord) HasTimeBounds() bool {
	return sr.mint <= sr.maxt
}

// createRecordCopy create a copy of the [Record].
func createSerializedRecordCopy(r *SerializedRecord) *SerializedRecord {
	c := *r
	return &c
}

// fullFields returns all the known fields of the record: the deletion time is known if it is set,
// the time bounds are known if they are valid.
func fullFields(sr *SerializedRecord) fieldMask {
	fields := fieldsAll
	if sr.deletedAt == 0 {
		fields &^= fieldDeletedAt
	}

	if !sr.HasTimeBounds() {
		fields &^= fieldMinT | fieldMaxT
	}

	return fields
}

// diffFields returns the fields whose values differ between the records. The segments count is not compared:
// it is changed in memory by the WAL writer concurrently with the catalog and is never a part of a change.
func diffFields(old, changed *SerializedRecord) fieldMask {
	var fields fieldMask
	if old.numberOfShards != changed.numberOfShards {
		fields |= fieldNumberOfShards
	}

	if old.createdAt != changed.createdAt {
		fields |= fieldCreatedAt
	}

	if old.updatedAt != changed.updatedAt {
		fields |= fieldUpdatedAt
	}

	if old.deletedAt != changed.deletedAt {
		fields |= fieldDeletedAt
	}

	if old.corrupted != changed.corrupted {
		fields |= fieldCorrupted
	}

	if old.status != changed.status {
		fields |= fieldStatus
	}

	if old.mint != changed.mint {
		fields |= fieldMinT
	}

	if old.maxt != changed.maxt {
		fields |= fieldMaxT
	}

	return fields
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
		SerializedRecord: SerializedRecord{
			mint: math.MaxInt64,
			maxt: math.MinInt64,
		},
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
		segmentsLock:    &sync.RWMutex{},
	}
}

// NewRecordWithData init new [Record] with parameters, all the known fields are present.
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
	r := &Record{
		SerializedRecord: SerializedRecord{
			id:                    id,
			numberOfShards:        numberOfShards,
			createdAt:             createdAt,
			updatedAt:             updatedAt,
			deletedAt:             deletedAt,
			corrupted:             corrupted,
			status:                status,
			lastAppendedSegmentID: optional.WithRawValue(lastAppendedSegmentID),
			numberOfSegments:      numberOfSegmentsByLastAppendedSegmentID(lastAppendedSegmentID),
			mint:                  math.MaxInt64,
			maxt:                  math.MinInt64,
		},
		referenceCount: referenceCount,
		// marking up through segment IDs by shards
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
		segmentsLock:    &sync.RWMutex{},
	}
	r.fields = fullFields(&r.SerializedRecord)

	return r
}

// NewRecordWithDataV3 init new [Record] version 3 with parameters, all the known fields are present.
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
	r := &Record{
		SerializedRecord: SerializedRecord{
			id:                    id,
			numberOfShards:        numberOfShards,
			createdAt:             createdAt,
			updatedAt:             updatedAt,
			deletedAt:             deletedAt,
			corrupted:             corrupted,
			status:                status,
			numberOfSegments:      numberOfSegments,
			lastAppendedSegmentID: lastAppendedSegmentIDByNumberOfSegments(numberOfSegments),
			mint:                  mint,
			maxt:                  maxt,
		},
		// marking up through segment IDs by shards
		lastSegmentID:   math.MaxUint32,
		segmentsByShard: make([]uint16, defaultSegmentsCapacity),
		segmentsLock:    &sync.RWMutex{},
	}
	r.fields = fullFields(&r.SerializedRecord)

	return r
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

// IsMissingSegmentsByShard returns true if there are missing segments by shard,
// i.e. some through segment ID is not marked while a greater one is.
func (r *Record) IsMissingSegmentsByShard() bool {
	for i := 1; i < len(r.segmentsByShard); i++ {
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

// RetentionTimestamp returns the timestamp from which the retention of the [Head] is counted:
// the max timestamp of the data if the time bounds are known, otherwise the creation time.
//
// The max timestamp comes from the samples, not from the wall clock, so it is clamped:
//   - to the update time from above: a sample from the future must not keep the head forever;
//   - to the creation time from below: backfilled old data must not remove the head before
//     the retention period since its creation has passed, e.g. before the remote writer has sent it.
func (r *Record) RetentionTimestamp() int64 {
	if !r.HasTimeBounds() {
		return r.createdAt
	}

	return max(r.createdAt, min(r.maxt, r.updatedAt))
}

// SetLastAppendedSegmentID set last appended segment id, keeps the number of segments in sync.
//
//go:norace
func (r *Record) SetLastAppendedSegmentID(segmentID uint32) {
	r.lastAppendedSegmentID.Set(segmentID)
	r.numberOfSegments = segmentID + 1
}

// SetNumberOfSegments number of segments in [Head], keeps the last appended segment id in sync.
//
//go:norace
func (r *Record) SetNumberOfSegments(numberOfSegments uint32) {
	r.numberOfSegments = numberOfSegments
	r.lastAppendedSegmentID = lastAppendedSegmentIDByNumberOfSegments(numberOfSegments)
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

// applyRecordChanges applies the fields of changed to the record, keeps the last appended segment id
// in sync with the segments count.
//
//go:norace
//revive:disable-next-line:cyclomatic // one branch per field.
func applyRecordChanges(sr, changed *SerializedRecord, fields fieldMask) {
	if fields.has(fieldNumberOfShards) {
		sr.numberOfShards = changed.numberOfShards
	}

	if fields.has(fieldCreatedAt) {
		sr.createdAt = changed.createdAt
	}

	if fields.has(fieldUpdatedAt) {
		sr.updatedAt = changed.updatedAt
	}

	if fields.has(fieldDeletedAt) {
		sr.deletedAt = changed.deletedAt
	}

	if fields.has(fieldCorrupted) {
		sr.corrupted = changed.corrupted
	}

	if fields.has(fieldStatus) {
		sr.status = changed.status
	}

	if fields.has(fieldSegmentsCount) {
		sr.numberOfSegments = changed.numberOfSegments
		sr.lastAppendedSegmentID = lastAppendedSegmentIDByNumberOfSegments(changed.numberOfSegments)
	}

	if fields.has(fieldMinT) {
		sr.mint = changed.mint
	}

	if fields.has(fieldMaxT) {
		sr.maxt = changed.maxt
	}
}

// lastAppendedSegmentIDByNumberOfSegments converts the number of segments to the last appended segment id.
func lastAppendedSegmentIDByNumberOfSegments(numberOfSegments uint32) optional.Optional[uint32] {
	var lastAppendedSegmentID optional.Optional[uint32]
	if numberOfSegments > 0 {
		lastAppendedSegmentID.Set(numberOfSegments - 1)
	}

	return lastAppendedSegmentID
}

// numberOfSegmentsByLastAppendedSegmentID converts the last appended segment id to the number of segments.
func numberOfSegmentsByLastAppendedSegmentID(lastAppendedSegmentID *uint32) uint32 {
	if lastAppendedSegmentID == nil {
		return 0
	}

	return *lastAppendedSegmentID + 1
}

// LessByUpdateAt less [Record] by UpdateAt.
func LessByUpdateAt(lhs, rhs *Record) bool {
	return lhs.UpdatedAt() < rhs.UpdatedAt()
}
