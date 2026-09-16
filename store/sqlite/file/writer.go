package file

import (
	"io"

	"github.com/mtlynch/picoshare/picoshare"
	"github.com/mtlynch/picoshare/store/sqlite/wrapped"
)

type writer struct {
	ctx            wrapped.SqlDB
	entryID        picoshare.EntryID
	buf            []byte
	written        uint64
	updateExisting bool
}

// Create a new writer for the entry ID using the given SqlTx and splitting the
// file into separate rows in the DB of at most chunkSize bytes.
func NewWriter(ctx wrapped.SqlDB, id picoshare.EntryID, chunkSize uint64) io.WriteCloser {
	return newWriterAt(ctx, id, chunkSize, 0, nil, false)
}

func NewWriterAt(
	ctx wrapped.SqlDB,
	id picoshare.EntryID,
	chunkSize, offset uint64,
	initialChunk []byte,
) io.WriteCloser {
	return newWriterAt(ctx, id, chunkSize, offset, initialChunk, true)
}

func newWriterAt(
	ctx wrapped.SqlDB,
	id picoshare.EntryID,
	chunkSize, offset uint64,
	initialChunk []byte,
	updateExisting bool,
) io.WriteCloser {
	buf := make([]byte, chunkSize)
	copy(buf, initialChunk)
	return new(writer{
		ctx:            ctx,
		entryID:        id,
		buf:            buf,
		written:        offset,
		updateExisting: updateExisting,
	})
}

// Write writes a buffer to the SQLite database.
func (w *writer) Write(p []byte) (int, error) {
	n := 0

	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	for {
		if n == len(p) {
			break
		}
		dstStart := int(w.written % uint64(len(w.buf)))
		copySize := min(len(w.buf)-dstStart, len(p)-n)
		dstEnd := dstStart + copySize
		copy(w.buf[dstStart:dstEnd], p[n:n+copySize])
		if dstEnd == len(w.buf) {
			if err := w.flush(len(w.buf)); err != nil {
				return n, err
			}
		}
		w.written += uint64(copySize)
		n += copySize
	}

	return n, nil
}

func (w *writer) Close() error {
	unflushed := int(w.written % uint64(len(w.buf)))
	if unflushed != 0 {
		return w.flush(unflushed)
	}
	return nil
}

func (w *writer) flush(n int) error {
	idx := int(w.written / uint64(len(w.buf)))
	query := `
	INSERT INTO
		entries_data
	(
		id,
		chunk_index,
		chunk
	)
	VALUES(?,?,?)`
	if w.updateExisting {
		query += `
	ON CONFLICT(id, chunk_index) DO UPDATE SET chunk=excluded.chunk`
	}

	_, err := w.ctx.Exec(query,
		w.entryID,
		idx,
		w.buf[0:n])

	return err
}
