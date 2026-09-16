package garbagecollect_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/mtlynch/picoshare/garbagecollect"
	"github.com/mtlynch/picoshare/picoshare"
	"github.com/mtlynch/picoshare/store"
	"github.com/mtlynch/picoshare/store/test_sqlite"
)

func TestCollectDoesNothingWhenStoreIsEmpty(t *testing.T) {
	dataStore := test_sqlite.New(t)
	c := garbagecollect.NewCollector(dataStore)
	err := c.Collect()
	if err != nil {
		t.Fatalf("garbage collection failed: %v", err)
	}

	remaining, err := dataStore.GetEntriesMetadata()
	if err != nil {
		t.Fatalf("retrieving datastore metadata failed: %v", err)
	}

	expected := []picoshare.UploadMetadata{}
	if !reflect.DeepEqual(expected, remaining) {
		t.Fatalf("unexpected results in datastore: got %+v, want %+v", remaining, expected)
	}
}

func TestCollectStaleChunkedUpload(t *testing.T) {
	dataStore := test_sqlite.New(t)
	uploadID := picoshare.EntryID("PENDING001")
	if err := dataStore.StartEntryUpload(picoshare.UploadMetadata{
		ID:          uploadID,
		Filename:    picoshare.Filename("partial.txt"),
		ContentType: picoshare.ContentType("text/plain"),
		Uploaded:    mustParseTime("2024-12-30T00:00:00Z"),
		Expires:     picoshare.NeverExpire,
		Size:        mustParseFileSize(7),
	}); err != nil {
		t.Fatalf("failed to start chunked upload: %v", err)
	}

	complete, err := dataStore.AppendEntryUpload(store.EntryUploadChunk{
		ID:        uploadID,
		Offset:    0,
		Length:    3,
		TotalSize: 7,
		Reader:    strings.NewReader("abc"),
	})
	if err != nil {
		t.Fatalf("failed to save partial upload: %v", err)
	}
	if got, want := complete, false; got != want {
		t.Fatalf("upload complete=%t, want=%t", got, want)
	}

	c := garbagecollect.NewCollector(dataStore)
	if err := c.Collect(); err != nil {
		t.Fatalf("garbage collection failed: %v", err)
	}

	if _, err := dataStore.GetEntryMetadata(uploadID); err == nil {
		t.Errorf("stale chunked upload remains visible")
	}
	remaining, err := dataStore.GetEntriesMetadata()
	if err != nil {
		t.Fatalf("failed to retrieve remaining entries: %v", err)
	}
	if got, want := len(remaining), 0; got != want {
		t.Errorf("remaining entry count=%d, want=%d", got, want)
	}
}

func TestCollectExpiredFile(t *testing.T) {
	dataStore := test_sqlite.New(t)
	d := "dummy data"
	expireInFiveMins := mustParseExpirationTime("2025-01-01T00:05:00Z")
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("AAAAAAAAAAAA"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("2024-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntryDownload(
		picoshare.EntryID("AAAAAAAAAAAA"),
		picoshare.DownloadRecord{
			Time:      mustParseTime("2023-06-01T12:00:00Z"),
			ClientIP:  "192.168.1.1",
			UserAgent: "test-agent",
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("BBBBBBBBBBBB"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("3000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("CCCCCCCCCCCC"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  picoshare.NeverExpire,
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("DDDDDDDDDDDD"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("2024-12-31T23:59:59Z"),
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("EEEEEEEEEEEE"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  expireInFiveMins,
			Size:     mustParseFileSize(len(d)),
		})

	c := garbagecollect.NewCollector(dataStore)
	err := c.Collect()
	if err != nil {
		t.Fatalf("garbage collection failed: %v", err)
	}

	remaining, err := dataStore.GetEntriesMetadata()
	if err != nil {
		t.Fatalf("retrieving datastore metadata failed: %v", err)
	}

	expected := []picoshare.UploadMetadata{
		{
			ID:       picoshare.EntryID("BBBBBBBBBBBB"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("3000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		},
		{
			ID:       picoshare.EntryID("CCCCCCCCCCCC"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  picoshare.NeverExpire,
			Size:     mustParseFileSize(len(d)),
		},
		{
			ID:       picoshare.EntryID("EEEEEEEEEEEE"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  expireInFiveMins,
			Size:     mustParseFileSize(len(d)),
		},
	}
	if diff := deep.Equal(expected, remaining); diff != nil {
		t.Errorf("unexpected results in datastore: got %v, want %v, diff = %v", remaining, expected, diff)
		t.Errorf("got=%+v", remaining)
		t.Errorf("want=%+v", expected)
		t.Errorf("diff=%+v", diff)
		t.FailNow()
	}
}

func TestCollectDoesNothingWhenNoFilesAreExpired(t *testing.T) {
	dataStore := test_sqlite.New(t)
	d := "dummy data"
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("AAAAAAAAAAAA"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("4000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("BBBBBBBBBBBB"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("3000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		})
	dataStore.InsertEntry(strings.NewReader(d),
		picoshare.UploadMetadata{
			ID:       picoshare.EntryID("CCCCCCCCCCCC"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  picoshare.NeverExpire,
			Size:     mustParseFileSize(len(d)),
		})

	c := garbagecollect.NewCollector(dataStore)
	err := c.Collect()
	if err != nil {
		t.Fatalf("garbage collection failed: %v", err)
	}

	remaining, err := dataStore.GetEntriesMetadata()
	if err != nil {
		t.Fatalf("retrieving datastore metadata failed: %v", err)
	}

	// Sort the elements so they have a consistent ordering.
	sort.Slice(remaining, func(i, j int) bool {
		return (time.Time(remaining[i].Expires)).After(time.Time(remaining[j].Expires))
	})

	expected := []picoshare.UploadMetadata{
		{
			ID:       picoshare.EntryID("AAAAAAAAAAAA"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("4000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		},
		{
			ID:       picoshare.EntryID("BBBBBBBBBBBB"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  mustParseExpirationTime("3000-01-01T00:00:00Z"),
			Size:     mustParseFileSize(len(d)),
		},
		{
			ID:       picoshare.EntryID("CCCCCCCCCCCC"),
			Uploaded: mustParseTime("2023-01-01T00:00:00Z"),
			Expires:  picoshare.NeverExpire,
			Size:     mustParseFileSize(len(d)),
		},
	}

	if diff := deep.Equal(expected, remaining); diff != nil {
		t.Errorf("unexpected results in datastore: got %v, want %v, diff = %v", remaining, expected, diff)
		t.Errorf("got=%+v", remaining)
		t.Errorf("want=%+v", expected)
		t.Errorf("diff=%+v", diff)
		t.FailNow()
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mustParseExpirationTime(s string) picoshare.ExpirationTime {
	et, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return picoshare.ExpirationTime(et)
}

func mustParseFileSize(val int) picoshare.FileSize {
	fileSize, err := picoshare.FileSizeFromInt(val)
	if err != nil {
		panic(err)
	}

	return fileSize
}
