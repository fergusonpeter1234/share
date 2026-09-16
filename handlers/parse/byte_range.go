package parse

import (
	"errors"
	"strconv"
	"strings"
)

var ErrInvalidByteRange = errors.New("invalid byte range")

type ByteRange struct {
	start uint64
	end   uint64
	total uint64
}

func ParseByteRange(raw string) (ByteRange, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bytes") {
		return ByteRange{}, ErrInvalidByteRange
	}

	rangePart, totalPart, ok := strings.Cut(parts[1], "/")
	if !ok || strings.Contains(totalPart, "/") {
		return ByteRange{}, ErrInvalidByteRange
	}

	startPart, endPart, ok := strings.Cut(rangePart, "-")
	if !ok || strings.Contains(endPart, "-") {
		return ByteRange{}, ErrInvalidByteRange
	}

	start, err := strconv.ParseUint(startPart, 10, 64)
	if err != nil {
		return ByteRange{}, ErrInvalidByteRange
	}

	end, err := strconv.ParseUint(endPart, 10, 64)
	if err != nil {
		return ByteRange{}, ErrInvalidByteRange
	}

	total, err := strconv.ParseUint(totalPart, 10, 64)
	if err != nil || total == 0 || start > end || end >= total {
		return ByteRange{}, ErrInvalidByteRange
	}

	return ByteRange{
		start: start,
		end:   end,
		total: total,
	}, nil
}

func (r ByteRange) Start() uint64 {
	return r.start
}

func (r ByteRange) End() uint64 {
	return r.end
}

func (r ByteRange) Total() uint64 {
	return r.total
}

func (r ByteRange) Length() uint64 {
	return r.end - r.start + 1
}
