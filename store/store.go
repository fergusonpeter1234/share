package store

import (
	"fmt"
	"io"

	"github.com/mtlynch/picoshare/picoshare"
)

// MaximumEntryUploadChunkSize keeps each HTTP upload request below common
// reverse-proxy request-size limits.
const MaximumEntryUploadChunkSize = uint64(50 * 1024 * 1024)

// EntryUploadChunk contains one sequential portion of an entry upload.
type EntryUploadChunk struct {
	ID          picoshare.EntryID
	GuestLinkID picoshare.GuestLinkID
	Offset      uint64
	Length      uint64
	TotalSize   uint64
	Reader      io.Reader
}

// EntryUploadInvalidError indicates that a chunked upload request is invalid.
type EntryUploadInvalidError struct {
	Err error
}

func (e EntryUploadInvalidError) Error() string {
	if e.Err == nil {
		return "invalid entry upload"
	}
	return fmt.Sprintf("invalid entry upload: %v", e.Err)
}

func (e EntryUploadInvalidError) Unwrap() error {
	return e.Err
}

// EntryUploadOffsetError indicates that a chunk did not start at the next
// expected byte.
type EntryUploadOffsetError struct {
	Expected uint64
	Received uint64
}

func (e EntryUploadOffsetError) Error() string {
	return fmt.Sprintf("unexpected upload offset: got %d, want %d", e.Received, e.Expected)
}

// EntryNotFoundError occurs when no entry exists with the given ID.
type EntryNotFoundError struct {
	ID picoshare.EntryID
}

func (f EntryNotFoundError) Error() string {
	return fmt.Sprintf("Could not find entry with ID %v", f.ID)
}

// GuestLinkNotFoundError occurs when no guest link exists with the given ID.
type GuestLinkNotFoundError struct {
	ID picoshare.GuestLinkID
}

func (f GuestLinkNotFoundError) Error() string {
	return fmt.Sprintf("Could not find guest link with ID %v", f.ID)
}
