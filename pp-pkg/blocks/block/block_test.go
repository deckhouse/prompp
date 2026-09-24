package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
)

// In Prometheus 2.1.0 we had a bug where the meta.json version was falsely bumped
// to 2. We had a migration in place resetting it to 1 but we should move immediately to
// version 3 next time to avoid confusion and issues.
func TestBlockMetaMustNeverBeVersion2(t *testing.T) {
	dir := t.TempDir()

	_, err := WriteTSDBMetaFile(log.NewNopLogger(), dir, &tsdb.BlockMeta{})
	require.NoError(t, err)

	meta, _, err := ReadFromDir(dir)
	require.NoError(t, err)
	require.NotEqual(t, 2, meta.Version, "meta.json version must never be 2")
}

func TestThanosMetaMustNeverBeVersion2(t *testing.T) {
	dir := t.TempDir()

	_, err := WriteThanosMetaFile(log.NewNopLogger(), dir, &metadata.Meta{})
	require.NoError(t, err)

	meta, _, err := ReadFromDir(dir)
	require.NoError(t, err)
	require.NotEqual(t, 2, meta.Version, "meta.json version must never be 2")
}

func TestSetCompactionFailed(t *testing.T) {
	tmpdir := t.TempDir()

	blockDir := testutils.CreateBlock(t, tmpdir, testutils.GenSeries(1, 1, 0, 1))
	b, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	require.False(t, b.meta.Compaction.Failed)
	require.NoError(t, b.SetCompactionFailed(WriteTSDBMetaFile))
	require.True(t, b.meta.Compaction.Failed)
	require.NoError(t, b.Close())

	b, err = OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	require.True(t, b.meta.Compaction.Failed)
	require.NoError(t, b.Close())
}

func TestCreateBlock(t *testing.T) {
	tmpdir := t.TempDir()
	b, err := OpenBlock(nil, testutils.CreateBlock(t, tmpdir, testutils.GenSeries(1, 1, 0, 10)), nil)
	require.NoError(t, err)
	require.NoError(t, b.Close())
}

func TestCorruptedChunk(t *testing.T) {
	for _, tc := range []struct {
		name     string
		corrFunc func(f *os.File) // Func that applies the corruption.
		openErr  error
		iterErr  error
	}{
		{
			name: "invalid header size",
			corrFunc: func(f *os.File) {
				require.NoError(t, f.Truncate(1))
			},
			openErr: errors.New("invalid segment header in segment 0: invalid size"),
		},
		{
			name: "invalid magic number",
			corrFunc: func(f *os.File) {
				magicChunksOffset := int64(0)
				_, err := f.Seek(magicChunksOffset, 0)
				require.NoError(t, err)

				// Set invalid magic number.
				b := make([]byte, chunks.MagicChunksSize)
				binary.BigEndian.PutUint32(b[:chunks.MagicChunksSize], 0x00000000)
				n, err := f.Write(b)
				require.NoError(t, err)
				require.Equal(t, chunks.MagicChunksSize, n)
			},
			openErr: errors.New("invalid magic number 0"),
		},
		{
			name: "invalid chunk format version",
			corrFunc: func(f *os.File) {
				chunksFormatVersionOffset := int64(4)
				_, err := f.Seek(chunksFormatVersionOffset, 0)
				require.NoError(t, err)

				// Set invalid chunk format version.
				b := make([]byte, chunks.ChunksFormatVersionSize)
				b[0] = 0
				n, err := f.Write(b)
				require.NoError(t, err)
				require.Equal(t, chunks.ChunksFormatVersionSize, n)
			},
			openErr: errors.New("invalid chunk format version 0"),
		},
		{
			name: "chunk not enough bytes to read the chunk length",
			corrFunc: func(f *os.File) {
				// Truncate one byte after the segment header.
				require.NoError(t, f.Truncate(chunks.SegmentHeaderSize+1))
			},
			iterErr: errors.New("cannot populate chunk 8 from block 00000000000000000000000000: " +
				"segment doesn't include enough bytes to read the chunk size data field - required:13, available:9"),
		},
		{
			name: "chunk not enough bytes to read the data",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				require.NoError(t, err)
				require.NoError(t, f.Truncate(fi.Size()-1))
			},
			iterErr: errors.New("cannot populate chunk 8 from block 00000000000000000000000000: " +
				"segment doesn't include enough bytes to read the chunk - required:26, available:25"),
		},
		{
			name: "checksum mismatch",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				require.NoError(t, err)

				// Get the chunk data end offset.
				chkEndOffset := int(fi.Size()) - crc32.Size

				// Seek to the last byte of chunk data and modify it.
				_, err = f.Seek(int64(chkEndOffset-1), 0)
				require.NoError(t, err)
				n, err := f.WriteString("x")
				require.NoError(t, err)
				require.Equal(t, 1, n)
			},
			iterErr: errors.New("cannot populate chunk 8 from block 00000000000000000000000000: " +
				"checksum mismatch expected:cfc0526c, actual:34815eae"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir := t.TempDir()

			series := storage.NewListSeries(
				labels.FromStrings("a", "b"),
				[]chunks.Sample{testutils.SampleTest{TS: 1, V: 1, HM: nil, FHM: nil}},
			)
			blockDir := testutils.CreateBlock(t, tmpdir, []storage.Series{series})
			files, err := sequenceFiles(ChunkDir(blockDir))
			require.NoError(t, err)
			require.NotEmpty(t, files, "No chunk created.")

			f, err := os.OpenFile(files[0], os.O_RDWR, 0o666)
			require.NoError(t, err)

			// Apply corruption function.
			tc.corrFunc(f)
			require.NoError(t, f.Close())

			// Check open err.
			b, err := OpenBlock(nil, blockDir, nil)
			if tc.openErr != nil {
				require.Equal(t, tc.openErr.Error(), err.Error())
				return
			}
			defer func() { require.NoError(t, b.Close()) }()

			querier, err := tsdb.NewBlockQuerier(b, 0, 1)
			require.NoError(t, err)
			defer func() { require.NoError(t, querier.Close()) }()
			set := querier.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			// Check chunk errors during iter time.
			require.True(t, set.Next())
			it := set.At().Iterator(nil)
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.Equal(t, tc.iterErr.Error(), it.Err().Error())
		})
	}
}

func TestLabelValuesWithMatchers(t *testing.T) {
	tmpdir := t.TempDir()
	ctx := t.Context()

	var seriesEntries []storage.Series
	for i := 0; i < 100; i++ {
		seriesEntries = append(seriesEntries, storage.NewListSeries(labels.FromStrings(
			"tens", fmt.Sprintf("value%d", i/10),
			"unique", fmt.Sprintf("value%d", i),
		), []chunks.Sample{testutils.SampleTest{TS: 100, V: 0, HM: nil, FHM: nil}}))
	}

	blockDir := testutils.CreateBlock(t, tmpdir, seriesEntries)
	files, err := sequenceFiles(ChunkDir(blockDir))
	require.NoError(t, err)
	require.NotEmpty(t, files, "No chunk created.")

	// Check open err.
	block, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, block.Close()) }()

	indexReader, err := block.Index()
	require.NoError(t, err)
	defer func() { require.NoError(t, indexReader.Close()) }()

	var uniqueWithout30s []string
	for i := 0; i < 100; i++ {
		if i/10 != 3 {
			uniqueWithout30s = append(uniqueWithout30s, fmt.Sprintf("value%d", i))
		}
	}
	sort.Strings(uniqueWithout30s)
	testCases := []struct {
		name           string
		labelName      string
		matchers       []*labels.Matcher
		expectedValues []string
	}{
		{
			name:           "get tens based on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value35")},
			expectedValues: []string{"value3"},
		}, {
			name:      "get unique ids based on a ten",
			labelName: "unique",
			matchers:  []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedValues: []string{
				"value10", "value11", "value12", "value13", "value14",
				"value15", "value16", "value17", "value18", "value19",
			},
		}, {
			name:           "get tens by pattern matching on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value[5-7]5")},
			expectedValues: []string{"value5", "value6", "value7"},
		}, {
			name:      "get tens by matching for presence of unique label",
			labelName: "tens",
			matchers:  []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedValues: []string{
				"value0", "value1", "value2", "value3", "value4", "value5", "value6", "value7", "value8", "value9",
			},
		}, {
			name:      "get unique IDs based on tens not being equal to a certain value, while not empty",
			labelName: "unique",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "tens", "value3"),
				labels.MustNewMatcher(labels.MatchNotEqual, "tens", ""),
			},
			expectedValues: uniqueWithout30s,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			actualValues, err := indexReader.SortedLabelValues(ctx, tt.labelName, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)

			actualValues, err = indexReader.LabelValues(ctx, tt.labelName, tt.matchers...)
			sort.Strings(actualValues)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)
		})
	}
}

func TestBlockSize(t *testing.T) {
	tmpdir := t.TempDir()

	var (
		blockInit    *Block
		expSizeInit  int64
		blockDirInit string
		err          error
	)

	// Create a block and compare the reported size vs actual disk size.
	{
		blockDirInit = testutils.CreateBlock(t, tmpdir, testutils.GenSeries(10, 1, 1, 100))
		blockInit, err = OpenBlock(nil, blockDirInit, nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, blockInit.Close())
		}()
		expSizeInit = blockInit.Size()
		actSizeInit, err := fileutil.DirSize(blockInit.Dir())
		require.NoError(t, err)
		require.Equal(t, expSizeInit, actSizeInit)
	}
}

func TestLabelNamesWithMatchers(t *testing.T) {
	tmpdir := t.TempDir()
	ctx := t.Context()

	var seriesEntries []storage.Series
	for i := range 100 {
		seriesEntries = append(seriesEntries, storage.NewListSeries(labels.FromStrings(
			"unique", fmt.Sprintf("value%d", i),
		), []chunks.Sample{testutils.SampleTest{TS: 100, V: 0, HM: nil, FHM: nil}}))

		if i%10 == 0 {
			seriesEntries = append(seriesEntries, storage.NewListSeries(labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"unique", fmt.Sprintf("value%d", i),
			), []chunks.Sample{testutils.SampleTest{TS: 100, V: 0, HM: nil, FHM: nil}}))
		}

		if i%20 == 0 {
			seriesEntries = append(seriesEntries, storage.NewListSeries(labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"twenties", fmt.Sprintf("value%d", i/20),
				"unique", fmt.Sprintf("value%d", i),
			), []chunks.Sample{testutils.SampleTest{TS: 100, V: 0, HM: nil, FHM: nil}}))
		}
	}

	blockDir := testutils.CreateBlock(t, tmpdir, seriesEntries)
	files, err := sequenceFiles(ChunkDir(blockDir))
	require.NoError(t, err)
	require.NotEmpty(t, files, "No chunk created.")

	// Check open err.
	block, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, block.Close()) })

	indexReader, err := block.Index()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, indexReader.Close()) })

	testCases := []struct {
		name          string
		labelName     string
		matchers      []*labels.Matcher
		expectedNames []string
	}{
		{
			name:          "get with non-empty unique: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get with unique ending in 1: only unique",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value.*1")},
			expectedNames: []string{"unique"},
		}, {
			name:          "get with unique = value20: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value20")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get tens = 1: unique & tens",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedNames: []string{"tens", "unique"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			actualNames, err := indexReader.LabelNames(ctx, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualNames)
		})
	}
}

func TestBlockIndexReader_PostingsForLabelMatching(t *testing.T) {
	testPostingsForLabelMatching(t, 2, func(t *testing.T, series []labels.Labels) tsdb.IndexReader {
		var seriesEntries []storage.Series
		for _, s := range series {
			seriesEntries = append(
				seriesEntries,
				storage.NewListSeries(s, []chunks.Sample{testutils.SampleTest{TS: 100, V: 0, HM: nil, FHM: nil}}),
			)
		}

		blockDir := testutils.CreateBlock(t, t.TempDir(), seriesEntries)
		files, err := sequenceFiles(ChunkDir(blockDir))
		require.NoError(t, err)
		require.NotEmpty(t, files, "No chunk created.")

		block, err := OpenBlock(nil, blockDir, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, block.Close()) })

		ir, err := block.Index()
		require.NoError(t, err)
		return ir
	})
}

func testPostingsForLabelMatching(
	t *testing.T,
	offset storage.SeriesRef,
	setUp func(*testing.T, []labels.Labels) tsdb.IndexReader,
) {
	t.Helper()

	ctx := t.Context()
	series := []labels.Labels{
		labels.FromStrings("n", "1"),
		labels.FromStrings("n", "1", "i", "a"),
		labels.FromStrings("n", "1", "i", "b"),
		labels.FromStrings("n", "2"),
		labels.FromStrings("n", "2.5"),
	}
	ir := setUp(t, series)
	t.Cleanup(func() {
		require.NoError(t, ir.Close())
	})

	testCases := []struct {
		name      string
		labelName string
		match     func(string) bool
		exp       []storage.SeriesRef
	}{
		{
			name:      "n=1",
			labelName: "n",
			match: func(val string) bool {
				return val == "1"
			},
			exp: []storage.SeriesRef{offset + 1, offset + 2, offset + 3},
		},
		{
			name:      "n=2",
			labelName: "n",
			match: func(val string) bool {
				return val == "2"
			},
			exp: []storage.SeriesRef{offset + 4},
		},
		{
			name:      "missing label",
			labelName: "missing",
			match: func(string) bool {
				return true
			},
			exp: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := ir.PostingsForLabelMatching(ctx, tc.labelName, tc.match)
			require.NotNil(t, p)
			srs, err := index.ExpandPostings(p)
			require.NoError(t, err)
			require.Equal(t, tc.exp, srs)
		})
	}
}

//
// Benchmark
//

func BenchmarkOpenBlock(b *testing.B) {
	tmpdir := b.TempDir()
	blockDir := testutils.CreateBlock(b, tmpdir, testutils.GenSeries(1e6, 20, 0, 10))
	b.Run("benchmark", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			block, err := OpenBlock(nil, blockDir, nil)
			require.NoError(b, err)
			require.NoError(b, block.Close())
		}
	})
}

//
// Test Helpers
//

// sequenceFiles returns the sequence files in the given directory.
func sequenceFiles(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	res := make([]string, 0, len(files))
	for _, fi := range files {
		if _, err := strconv.ParseUint(fi.Name(), 10, 64); err != nil {
			continue
		}
		res = append(res, filepath.Join(dir, fi.Name()))
	}

	return res, nil
}
