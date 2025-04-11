package block

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
	"github.com/prometheus/prometheus/util/pool"
)

// headerStreamSize header size for stream.
const headerReaderSize = 4 + 4

// Reader is segments block reader.
type Reader struct {
	file                  *os.File
	blockHeader           storage.BlockHeader
	lastReadSegmentHeader storage.SegmentHeader
	buffers               *pool.Pool
}

// Header return header of block.
func (r *Reader) Header() storage.BlockHeader {
	return r.blockHeader
}

// Next read and return next segment.
func (r *Reader) Next() (*model.Segment, error) {
	header := r.buffers.Get(headerReaderSize).([]byte)
	model.ResizeBuffer(headerReaderSize, &header)

	bytesRead, err := io.ReadFull(r.file, header)
	if err != nil {
		r.buffers.Put(header)
		if errors.Is(err, io.EOF) {
			return nil, storage.ErrEndOfBlock
		}

		return nil, fmt.Errorf("failed to read segment header: %w", err)
	}

	if bytesRead != headerReaderSize {
		r.buffers.Put(header)
		return nil, fmt.Errorf(
			"failed to read segment header, bytes read: %d, header size: %d",
			bytesRead,
			headerReaderSize,
		)
	}

	segmentHeader := storage.SegmentHeader{
		ID:    r.lastReadSegmentHeader.ID + 1,
		Size:  binary.LittleEndian.Uint32(header[:4]),
		CRC32: binary.LittleEndian.Uint32(header[4:8]),
	}
	r.buffers.Put(header)

	segment := &model.Segment{
		ID:   segmentHeader.ID,
		Size: segmentHeader.Size,
		CRC:  segmentHeader.CRC32,
		Body: r.buffers.Get(int(segmentHeader.Size)).([]byte),
	}
	model.ResizeBuffer(int(segmentHeader.Size), &segment.Body)
	if _, err = io.ReadFull(r.file, segment.Body); err != nil {
		r.buffers.Put(segment.Body)
		return nil, fmt.Errorf("failed to read segment body: %w", err)
	}

	r.lastReadSegmentHeader = segmentHeader
	segment.DestroyFn = func() { r.buffers.Put(segment.Body) }

	return segment, nil
}

// Close reader.
func (r *Reader) Close() error {
	return r.file.Close()
}
