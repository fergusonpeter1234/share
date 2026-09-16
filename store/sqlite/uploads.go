package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"

	"github.com/mtlynch/picoshare/picoshare"
	"github.com/mtlynch/picoshare/store"
	"github.com/mtlynch/picoshare/store/sqlite/file"
)

const (
	entryUploadStatePending  = "pending"
	entryUploadStateComplete = "complete"
)

func (s Store) StartEntryUpload(metadata picoshare.UploadMetadata) error {
	expectedSize := metadata.Size.UInt64()
	if expectedSize == 0 || expectedSize > maxSQLiteInteger {
		return store.EntryUploadInvalidError{
			Err: fmt.Errorf("file size cannot be stored in SQLite"),
		}
	}

	_, err := s.db.Exec(`
	INSERT INTO
		entries
	(
		id,
		guest_link_id,
		filename,
		note,
		content_type,
		upload_time,
		expiration_time,
		expected_size,
		upload_state
	)
	VALUES(
		:entry_id,
		NULLIF(:guest_link_id, ''),
		:filename,
		:note,
		:content_type,
		:upload_time,
		:expiration_time,
		:expected_size,
		:upload_state
	)`,
		sql.Named("entry_id", metadata.ID),
		sql.Named("guest_link_id", metadata.GuestLink.ID),
		sql.Named("filename", metadata.Filename),
		sql.Named("note", metadata.Note.Value),
		sql.Named("content_type", metadata.ContentType),
		sql.Named("upload_time", formatTime(metadata.Uploaded)),
		sql.Named("expiration_time", formatExpirationTime(metadata.Expires)),
		sql.Named("expected_size", expectedSize),
		sql.Named("upload_state", entryUploadStatePending),
	)
	return err
}

func (s Store) AppendEntryUpload(chunk store.EntryUploadChunk) (bool, error) {
	if err := validateEntryUploadChunk(chunk); err != nil {
		return false, store.EntryUploadInvalidError{Err: err}
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("failed to rollback entry upload: %v", err)
		}
	}()

	var uploadState string
	var expectedSize int64
	var guestLinkID sql.NullString
	var receivedSize int64
	err = tx.QueryRow(`
	SELECT
		upload_state,
		expected_size,
		guest_link_id,
		COALESCE(
			(
				SELECT SUM(LENGTH(chunk))
				FROM entries_data
				WHERE entries_data.id = entries.id
			),
			0
		)
	FROM
		entries
	WHERE
		entries.id = :entry_id`, sql.Named("entry_id", chunk.ID)).Scan(
		&uploadState,
		&expectedSize,
		&guestLinkID,
		&receivedSize,
	)
	if err == sql.ErrNoRows {
		return false, store.EntryNotFoundError{ID: chunk.ID}
	}
	if err != nil {
		return false, err
	}

	if uploadState != entryUploadStatePending {
		return false, store.EntryNotFoundError{ID: chunk.ID}
	}
	if expectedSize < 0 || receivedSize < 0 {
		return false, store.EntryUploadInvalidError{
			Err: fmt.Errorf("entry upload has an invalid size"),
		}
	}

	storedGuestLinkID := picoshare.GuestLinkID(guestLinkID.String)
	if storedGuestLinkID != chunk.GuestLinkID {
		return false, store.EntryNotFoundError{ID: chunk.ID}
	}

	expectedTotalSize := uint64(expectedSize)
	if chunk.TotalSize != expectedTotalSize {
		return false, store.EntryUploadInvalidError{
			Err: fmt.Errorf(
				"chunk total size does not match the upload size"),
		}
	}
	if uint64(receivedSize) != chunk.Offset {
		return false, store.EntryUploadOffsetError{
			Expected: uint64(receivedSize),
			Received: chunk.Offset,
		}
	}
	if chunk.Length > expectedTotalSize-chunk.Offset {
		return false, store.EntryUploadInvalidError{
			Err: fmt.Errorf("chunk extends beyond the upload size"),
		}
	}

	var initialChunk []byte
	chunkSize := uint64(s.chunkSize)
	if chunk.Offset%chunkSize != 0 {
		chunkIndex := chunk.Offset / chunkSize
		if err := tx.QueryRow(`
		SELECT
			chunk
		FROM
			entries_data
		WHERE
			id = :entry_id AND
			chunk_index = :chunk_index`,
			sql.Named("entry_id", chunk.ID),
			sql.Named("chunk_index", chunkIndex),
		).Scan(&initialChunk); err != nil {
			return false, store.EntryUploadInvalidError{
				Err: fmt.Errorf("failed to find the previous upload chunk: %w", err),
			}
		}

		partialChunkSize := chunk.Offset % chunkSize
		if uint64(len(initialChunk)) != partialChunkSize {
			return false, store.EntryUploadInvalidError{
				Err: fmt.Errorf("previous upload chunk has an invalid size"),
			}
		}
	}

	w := file.NewWriterAt(
		tx,
		chunk.ID,
		s.chunkSize,
		chunk.Offset,
		initialChunk,
	)
	bytesWritten, err := io.Copy(
		w,
		io.LimitReader(chunk.Reader, int64(chunk.Length)+1),
	)
	if err != nil {
		return false, store.EntryUploadInvalidError{
			Err: fmt.Errorf("failed to save upload chunk: %w", err),
		}
	}
	if bytesWritten != int64(chunk.Length) {
		return false, store.EntryUploadInvalidError{
			Err: fmt.Errorf("chunk body length does not match Content-Range"),
		}
	}
	if err := w.Close(); err != nil {
		return false, err
	}

	newReceivedSize := uint64(receivedSize) + chunk.Length
	newUploadState := entryUploadStatePending
	if newReceivedSize == expectedTotalSize {
		newUploadState = entryUploadStateComplete
	}
	if _, err := tx.Exec(`
	UPDATE entries
	SET
		upload_state = :upload_state
	WHERE
		id = :entry_id AND
		upload_state = :pending_state`,
		sql.Named("upload_state", newUploadState),
		sql.Named("entry_id", chunk.ID),
		sql.Named("pending_state", entryUploadStatePending)); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return newUploadState == entryUploadStateComplete, nil
}

func validateEntryUploadChunk(chunk store.EntryUploadChunk) error {
	if chunk.Reader == nil {
		return fmt.Errorf("chunk body is missing")
	}
	if chunk.TotalSize == 0 || chunk.Length == 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	if chunk.TotalSize > maxSQLiteInteger || chunk.Length >= maxSQLiteInteger {
		return fmt.Errorf("chunk size cannot be stored in SQLite")
	}
	if chunk.Length > store.MaximumEntryUploadChunkSize {
		return fmt.Errorf("chunk exceeds maximum size")
	}
	if chunk.Offset > chunk.TotalSize || chunk.Length > chunk.TotalSize-chunk.Offset {
		return fmt.Errorf("chunk range is invalid")
	}
	return nil
}
