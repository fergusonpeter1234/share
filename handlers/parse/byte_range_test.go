package parse_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mtlynch/picoshare/handlers/parse"
)

func TestParseByteRange(t *testing.T) {
	for _, tt := range []struct {
		explanation string
		input       string
		start       uint64
		end         uint64
		total       uint64
		errExpected error
	}{
		{
			explanation: "valid byte range is parsed",
			input:       "bytes 0-4/10",
			start:       0,
			end:         4,
			total:       10,
		},
		{
			explanation: "byte range unit is case insensitive",
			input:       "BYTES 5-9/10",
			start:       5,
			end:         9,
			total:       10,
		},
		{
			explanation: "range with missing total is rejected",
			input:       "bytes 0-4/*",
			errExpected: parse.ErrInvalidByteRange,
		},
		{
			explanation: "range with an end beyond the total is rejected",
			input:       "bytes 0-10/10",
			errExpected: parse.ErrInvalidByteRange,
		},
		{
			explanation: "range with reversed bounds is rejected",
			input:       "bytes 5-4/10",
			errExpected: parse.ErrInvalidByteRange,
		},
	} {
		t.Run(fmt.Sprintf("%s [%s]", tt.explanation, tt.input), func(t *testing.T) {
			r, err := parse.ParseByteRange(tt.input)
			if tt.errExpected != nil {
				if got, want := errors.Is(err, tt.errExpected), true; got != want {
					t.Fatalf("err=%v, want=%v", err, tt.errExpected)
				}
				return
			}
			if err != nil {
				t.Fatalf("err=%v, want nil", err)
			}

			if got, want := r.Start(), tt.start; got != want {
				t.Errorf("start=%d, want=%d", got, want)
			}
			if got, want := r.End(), tt.end; got != want {
				t.Errorf("end=%d, want=%d", got, want)
			}
			if got, want := r.Total(), tt.total; got != want {
				t.Errorf("total=%d, want=%d", got, want)
			}
			if got, want := r.Length(), tt.end-tt.start+1; got != want {
				t.Errorf("length=%d, want=%d", got, want)
			}
		})
	}
}
