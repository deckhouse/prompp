package block

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

const (
	// ChunksDirname is the known dir name for chunks with compressed samples.
	ChunksDirname = "chunks"

	// IndexFilename is the known index file for block index.
	IndexFilename = "index"

	// MetaFilename is the known meta file for block meta.
	MetaFilename = "meta.json"

	// MetaVersion1 is the known version of the block meta file.
	MetaVersion1 = 1

	// CompactionHintFromOutOfOrder is a hint noting that the block
	// was created from out-of-order chunks.
	CompactionHintFromOutOfOrder = "from-out-of-order"

	// CompactionHintCorrupted is a hint noting that the block is corrupted.
	CompactionHintCorrupted = "corrupted"
)

// ErrClosing is returned when a block is in the process of being closed.
var ErrClosing = errors.New("block is closing")

//
// Block
//

// Block represents a directory of time series data covering a continuous time range.
// Copied from [tsdb.Block] for convenience.
type Block struct {
	mtx            sync.RWMutex
	closing        bool
	pendingReaders sync.WaitGroup

	dir  string
	meta metadata.Meta

	// Symbol Table Size in bytes.
	// We maintain this variable to avoid recalculation every time.
	symbolTableSize uint64

	chunkr     tsdb.ChunkReader
	indexr     tsdb.IndexReader
	tombstones tombstones.Reader

	logger log.Logger

	numBytesChunks    int64
	numBytesIndex     int64
	numBytesTombstone int64
	numBytesMeta      int64
}

// OpenBlocks loads all blocks from dir, reusing already-loaded ones (usage: pp-pkg/blocks/manger).
func OpenBlocks(
	l log.Logger,
	dir string,
	loaded []*Block,
	chunkPool chunkenc.Pool,
) ([]*Block, map[ulid.ULID]error, error) {
	bDirs, err := DirsOfBlocks(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("find blocks: %w", err)
	}

	corrupted := make(map[ulid.ULID]error)
	blocks := make([]*Block, 0, len(bDirs))
	for _, bDir := range bDirs {
		meta, _, err := ReadFromDir(bDir)
		if err != nil {
			_ = level.Error(l).Log(
				"msg", "Failed to read meta.json for a block during reloadBlocks. Skipping",
				"dir", bDir,
				"err", err,
			)

			continue
		}

		// See if we already have the block in memory or open it otherwise.
		block, open := getBlock(loaded, meta.ULID)
		if !open {
			block, err = OpenBlock(l, bDir, chunkPool)
			if err != nil {
				corrupted[meta.ULID] = err
				continue
			}
		}

		if meta.Compaction.UnsetCorrupted() {
			// unmark block as corrupted
			if _, err := WriteThanosMetaFile(l, bDir, meta); err != nil {
				_ = level.Error(l).Log(
					"msg", "Failed to update meta.json for a block during reloadBlocks",
					"dir", bDir,
					"err", err,
				)
			}
		}

		blocks = append(blocks, block)
	}

	return blocks, corrupted, nil
}

// OpenBlock opens the block in the directory. It can be passed a chunk pool, which is used
// to instantiate chunk structs.
func OpenBlock(logger log.Logger, dir string, pool chunkenc.Pool) (pb *Block, err error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	var closers []io.Closer
	defer func() {
		if err != nil {
			err = errors.Join(err, CloseAll(closers))
		}
	}()

	meta, sizeMeta, err := ReadFromDir(dir)
	if err != nil {
		return nil, err
	}

	cr, err := chunks.NewDirReader(ChunkDir(dir), pool)
	if err != nil {
		return nil, err
	}
	closers = append(closers, cr)

	ir, err := index.NewFileReader(filepath.Join(dir, IndexFilename))
	if err != nil {
		return nil, err
	}
	closers = append(closers, ir)

	tr, sizeTomb, err := tombstones.ReadTombstones(dir)
	if err != nil {
		return nil, err
	}
	closers = append(closers, tr)

	return &Block{
		dir:               dir,
		meta:              *meta,
		chunkr:            cr,
		indexr:            ir,
		tombstones:        tr,
		symbolTableSize:   ir.SymbolTableSize(),
		logger:            logger,
		numBytesChunks:    cr.Size(),
		numBytesIndex:     ir.Size(),
		numBytesTombstone: sizeTomb,
		numBytesMeta:      sizeMeta,
	}, nil
}

// Chunks returns a new ChunkReader against the block data.
func (pb *Block) Chunks() (tsdb.ChunkReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}

	return blockChunkReader{ChunkReader: pb.chunkr, b: pb}, nil
}

// Close closes the on-disk block. It blocks as long as there are readers reading from the block.
func (pb *Block) Close() error {
	pb.mtx.Lock()
	pb.closing = true
	pb.mtx.Unlock()

	pb.pendingReaders.Wait()

	return errors.Join(
		pb.chunkr.Close(),
		pb.indexr.Close(),
		pb.tombstones.Close(),
	)
}

// Dir returns the directory of the block.
func (pb *Block) Dir() string { return pb.dir }

// GetSymbolTableSize returns the Symbol Table Size in the index of this block.
func (pb *Block) GetSymbolTableSize() uint64 {
	return pb.symbolTableSize
}

// Index returns a new IndexReader against the block data.
func (pb *Block) Index() (tsdb.IndexReader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}

	return blockIndexReader{ir: pb.indexr, b: pb}, nil
}

// LabelNames returns all the unique label names present in the Block in sorted order.
func (pb *Block) LabelNames(ctx context.Context) ([]string, error) {
	return pb.indexr.LabelNames(ctx)
}

// Meta returns [tsdb.BlockMeta] meta information about the block.
func (pb *Block) Meta() tsdb.BlockMeta { return pb.meta.BlockMeta }

// Metadata returns the thanos [metadata.Meta] of the block.
func (pb *Block) Metadata() *metadata.Meta { return &pb.meta }

// OverlapsClosedInterval returns true if the block overlaps [mint, maxt].
func (pb *Block) OverlapsClosedInterval(mint, maxt int64) bool {
	// The block itself is a half-open interval
	// [pb.meta.MinTime, pb.meta.MaxTime).
	return pb.meta.MinTime <= maxt && mint < pb.meta.MaxTime
}

// SetAsDeletable sets the block as deletable.
func (pb *Block) SetAsDeletable() {
	pb.meta.Compaction.Deletable = true
}

// SetCompactionFailed sets the block as compaction failed.
func (pb *Block) SetCompactionFailed(
	writeMetaFileFn func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error),
) error {
	pb.meta.Compaction.Failed = true
	n, err := writeMetaFileFn(pb.logger, pb.dir, &pb.meta.BlockMeta)
	if err != nil {
		return err
	}

	pb.numBytesMeta = n

	return nil
}

// SetNumBytesMeta sets the number of bytes of the meta file.
func (pb *Block) SetNumBytesMeta(n int64) {
	pb.numBytesMeta = n
}

// Size returns the number of bytes that the block takes up.
func (pb *Block) Size() int64 {
	return pb.numBytesChunks + pb.numBytesIndex + pb.numBytesTombstone + pb.numBytesMeta
}

// String returns the string representation of the block.
func (pb *Block) String() string {
	return pb.meta.ULID.String()
}

// Tombstones returns a new TombstoneReader against the block data.
func (pb *Block) Tombstones() (tombstones.Reader, error) {
	if err := pb.startRead(); err != nil {
		return nil, err
	}

	return blockTombstoneReader{Reader: pb.tombstones, b: pb}, nil
}

// startRead starts a read operation on the block.
func (pb *Block) startRead() error {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()

	if pb.closing {
		return ErrClosing
	}

	pb.pendingReaders.Add(1)

	return nil
}

//
// blockChunkReader
//

// blockChunkReader is a wrapper around [tsdb.ChunkReader] that implements [io.Closer].
// It is used to close the block when the chunk reader is closed.
type blockChunkReader struct {
	tsdb.ChunkReader
	b *Block
}

// Close closes the block chunk reader.
func (r blockChunkReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

//
// blockIndexReader
//

// blockIndexReader is a wrapper around [tsdb.IndexReader] that implements [io.Closer].
// It is used to close the block when the index reader is closed.
type blockIndexReader struct {
	ir tsdb.IndexReader
	b  *Block
}

// Close closes the block index reader.
func (r blockIndexReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

// LabelNames returns all the unique label names present in the Block in sorted order.
func (r blockIndexReader) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) == 0 {
		return r.b.LabelNames(ctx)
	}

	return labelNamesWithMatchers(ctx, r.ir, matchers...)
}

// LabelNamesFor returns all the label names for the series referred to by the postings.
// The names returned are sorted.
func (r blockIndexReader) LabelNamesFor(ctx context.Context, postings index.Postings) ([]string, error) {
	return r.ir.LabelNamesFor(ctx, postings)
}

// LabelValueFor returns label value for the given label name in the series referred to by ID.
func (r blockIndexReader) LabelValueFor(ctx context.Context, id storage.SeriesRef, label string) (string, error) {
	return r.ir.LabelValueFor(ctx, id, label)
}

// LabelValues returns all the label values for the series referred to by the postings.
// The values returned are sorted.
func (r blockIndexReader) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	if len(matchers) == 0 {
		st, err := r.ir.LabelValues(ctx, name)
		if err != nil {
			return st, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
		}
		return st, nil
	}

	return labelValuesWithMatchers(ctx, r.ir, name, matchers...)
}

// Postings returns the postings list iterator for the label pairs.
// The Postings here contain the offsets to the series inside the index.
// Found IDs are not strictly required to point to a valid Series, e.g.
// during background garbage collections.
func (r blockIndexReader) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	p, err := r.ir.Postings(ctx, name, values...)
	if err != nil {
		return p, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}

	return p, nil
}

// PostingsForLabelMatching returns a sorted iterator over postings having a label with
// the given name and a value for which match returns true.
// If no postings are found having at least one matching label, an empty iterator is returned.
func (r blockIndexReader) PostingsForLabelMatching(
	ctx context.Context,
	name string,
	match func(string) bool,
) index.Postings {
	return r.ir.PostingsForLabelMatching(ctx, name, match)
}

// Series populates the given builder and chunk metas for the series identified
// by the reference.
func (r blockIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	if err := r.ir.Series(ref, builder, chks); err != nil {
		//revive:disable-next-line:add-constant // copied from upstream
		return fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}

	return nil
}

// ShardedPostings returns a postings list filtered by the provided shardIndex
// out of shardCount. For a given posting, its shard MUST be computed hashing
// the series labels mod shardCount, using a hash function which is consistent over time.
func (r blockIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	return r.ir.ShardedPostings(p, shardIndex, shardCount)
}

// SortedLabelValues returns sorted possible label values.
func (r blockIndexReader) SortedLabelValues(
	ctx context.Context,
	name string,
	matchers ...*labels.Matcher,
) ([]string, error) {
	var st []string
	var err error

	if len(matchers) == 0 {
		st, err = r.ir.SortedLabelValues(ctx, name)
	} else {
		st, err = r.LabelValues(ctx, name, matchers...)
		if err == nil {
			slices.Sort(st)
		}
	}

	if err != nil {
		return st, fmt.Errorf("block: %s: %w", r.b.Meta().ULID, err)
	}

	return st, nil
}

// SortedPostings returns a postings list that is reordered to be sorted
// by the label set of the underlying series.
func (r blockIndexReader) SortedPostings(p index.Postings) index.Postings {
	return r.ir.SortedPostings(p)
}

// Symbols returns an iterator over sorted string symbols that may occur in
// series labels and indices. It is not safe to use the returned strings
// beyond the lifetime of the index reader.
func (r blockIndexReader) Symbols() index.StringIter {
	return r.ir.Symbols()
}

//
// blockTombstoneReader
//

// blockTombstoneReader is a wrapper around [tombstones.Reader] that implements [io.Closer].
// It is used to close the block when the tombstone reader is closed.
type blockTombstoneReader struct {
	tombstones.Reader
	b *Block
}

// Close closes the block tombstone reader.
func (r blockTombstoneReader) Close() error {
	r.b.pendingReaders.Done()
	return nil
}

//
// Overlaps
//

// Overlaps contains overlapping blocks aggregated by overlapping range.
type Overlaps map[TimeRange][]tsdb.BlockMeta

// TimeRange specifies minTime and maxTime range.
type TimeRange struct {
	Min, Max int64
	Key      string
}

// String returns human readable string form of overlapped blocks.
func (o Overlaps) String() string {
	var res []string
	for r, overlaps := range o {
		var groups []string
		for i := range overlaps {
			groups = append(groups, fmt.Sprintf(
				"<ulid: %s, mint: %d, maxt: %d, range: %s>",
				overlaps[i].ULID.String(),
				overlaps[i].MinTime,
				overlaps[i].MaxTime,
				(time.Duration((overlaps[i].MaxTime-overlaps[i].MinTime)/1000)*time.Second).String(),
			))
		}

		res = append(res, fmt.Sprintf(
			"[key: %s, mint: %d, maxt: %d, range: %s, blocks: %d]: %s",
			r.Key,
			r.Min, r.Max,
			(time.Duration((r.Max-r.Min)/1000)*time.Second).String(),
			len(overlaps),
			strings.Join(groups, ", ")),
		)
	}

	return strings.Join(res, "\n")
}
