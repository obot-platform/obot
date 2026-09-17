//nolint:revive
package time

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	parseDurationTests := []struct {
		in   string
		want time.Duration
	}{
		// These tests are pulled straight from the Go source
		// simple
		{
			in:   "0",
			want: 0,
		},
		{
			in:   "5s",
			want: 5 * time.Second,
		},
		{
			in:   "30s",
			want: 30 * time.Second,
		},
		{
			in:   "1478s",
			want: 1478 * time.Second,
		},
		// sign
		{
			in:   "-5s",
			want: -5 * time.Second,
		},
		{
			in:   "+5s",
			want: 5 * time.Second,
		},
		{
			in:   "-0",
			want: 0,
		},
		{
			in:   "+0",
			want: 0,
		},
		// decimal
		{
			in:   "5.0s",
			want: 5 * time.Second,
		},
		{
			in:   "5.6s",
			want: 5*time.Second + 600*time.Millisecond,
		},
		{
			in:   "5.s",
			want: 5 * time.Second,
		},
		{
			in:   ".5s",
			want: 500 * time.Millisecond,
		},
		{
			in:   "1.0s",
			want: 1 * time.Second,
		},
		{
			in:   "1.00s",
			want: 1 * time.Second,
		},
		{
			in:   "1.004s",
			want: 1*time.Second + 4*time.Millisecond,
		},
		{
			in:   "1.0040s",
			want: 1*time.Second + 4*time.Millisecond,
		},
		{
			in:   "100.00100s",
			want: 100*time.Second + 1*time.Millisecond,
		},
		// different units
		{
			in:   "10ns",
			want: 10 * time.Nanosecond,
		},
		{
			in:   "11us",
			want: 11 * time.Microsecond,
		},
		{
			in:   "12µs",
			want: 12 * time.Microsecond,
		}, // U+00B5
		{
			in:   "12μs",
			want: 12 * time.Microsecond,
		}, // U+03BC
		{
			in:   "13ms",
			want: 13 * time.Millisecond,
		},
		{
			in:   "14s",
			want: 14 * time.Second,
		},
		{
			in:   "15m",
			want: 15 * time.Minute,
		},
		{
			in:   "16h",
			want: 16 * time.Hour,
		},
		// composite durations
		{
			in:   "3h30m",
			want: 3*time.Hour + 30*time.Minute,
		},
		{
			in:   "10.5s4m",
			want: 4*time.Minute + 10*time.Second + 500*time.Millisecond,
		},
		{
			in:   "-2m3.4s",
			want: -(2*time.Minute + 3*time.Second + 400*time.Millisecond),
		},
		{
			in:   "1h2m3s4ms5us6ns",
			want: 1*time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond + 5*time.Microsecond + 6*time.Nanosecond,
		},
		{
			in:   "39h9m14.425s",
			want: 39*time.Hour + 9*time.Minute + 14*time.Second + 425*time.Millisecond,
		},
		// large value
		{
			in:   "52763797000ns",
			want: 52763797000 * time.Nanosecond,
		},
		// more than 9 digits after decimal point, see https://golang.org/issue/6617
		{
			in:   "0.3333333333333333333h",
			want: 20 * time.Minute,
		},
		// 9007199254740993 = 1<<53+1 cannot be stored precisely in a float64
		{
			in:   "9007199254740993ns",
			want: (1<<53 + 1) * time.Nanosecond,
		},
		// largest duration that can be represented by int64 in nanoseconds
		{
			in:   "9223372036854775807ns",
			want: (1<<63 - 1) * time.Nanosecond,
		},
		{
			in:   "9223372036854775.807us",
			want: (1<<63 - 1) * time.Nanosecond,
		},
		{
			in:   "9223372036s854ms775us807ns",
			want: (1<<63 - 1) * time.Nanosecond,
		},
		{
			in:   "-9223372036854775808ns",
			want: -1 << 63 * time.Nanosecond,
		},
		{
			in:   "-9223372036854775.808us",
			want: -1 << 63 * time.Nanosecond,
		},
		{
			in:   "-9223372036s854ms775us808ns",
			want: -1 << 63 * time.Nanosecond,
		},
		// largest negative value
		{
			in:   "-9223372036854775808ns",
			want: -1 << 63 * time.Nanosecond,
		},
		// largest negative round trip value, see https://golang.org/issue/48629
		{
			in:   "-2562047h47m16.854775808s",
			want: -1 << 63 * time.Nanosecond,
		},
		// huge string; issue 15011.
		{
			in:   "0.100000000000000000000h",
			want: 6 * time.Minute,
		},
		// This value tests the first overflow check in leadingFraction.
		{
			in:   "0.830103483285477580700h",
			want: 49*time.Minute + 48*time.Second + 372539827*time.Nanosecond,
		},

		// These tests test the new functionality added in this package.
		{
			in:   "1d",
			want: 24 * time.Hour,
		},
		{
			in:   "1w",
			want: 7 * 24 * time.Hour,
		},
		{
			in:   "4w3d",
			want: 31 * 24 * time.Hour,
		},
		{
			in:   "-5w7d",
			want: -42 * 24 * time.Hour,
		},
		{
			in:   "1w0.5d1h2m3s4ms5us6ns",
			want: (7*24+12+1)*time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond + 5*time.Microsecond + 6*time.Nanosecond,
		},
		{
			in:   "0.5w",
			want: 7 * 12 * time.Hour,
		},
	}
	for _, tc := range parseDurationTests {
		d, err := ParseDuration(tc.in)
		if err != nil || d != tc.want {
			t.Errorf("ParseDuration(%q) = %v, %v, want %v, nil", tc.in, d, err, tc.want)
		}
	}
}
