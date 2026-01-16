package server

import (
	"testing"
	"time"
)

func TestParseDateRange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		requestedStart string
		requestedEnd   string
		wantStartZero bool
		wantEndZero   bool
		wantErr       bool
	}{
		{
			name:          "both empty - defaults to last 24 hours",
			requestedStart: "",
			requestedEnd:   "",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
		{
			name:          "valid RFC3339 dates",
			requestedStart: "2024-01-15T10:00:00Z",
			requestedEnd:   "2024-01-16T10:00:00Z",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
		{
			name:          "only start provided - end defaults to now",
			requestedStart: "2024-01-15T10:00:00Z",
			requestedEnd:   "",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
		{
			name:          "only end provided - start defaults to 24h ago",
			requestedStart: "",
			requestedEnd:   "2024-01-16T10:00:00Z",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
		{
			name:          "invalid start date format",
			requestedStart: "not-a-date",
			requestedEnd:   "2024-01-16T10:00:00Z",
			wantStartZero: true,
			wantEndZero:   true,
			wantErr:       true,
		},
		{
			name:          "invalid end date format",
			requestedStart: "2024-01-15T10:00:00Z",
			requestedEnd:   "invalid",
			wantStartZero: true,
			wantEndZero:   true,
			wantErr:       true,
		},
		{
			name:          "both invalid",
			requestedStart: "bad-start",
			requestedEnd:   "bad-end",
			wantStartZero: true,
			wantEndZero:   true,
			wantErr:       true,
		},
		{
			name:          "RFC3339 with timezone offset",
			requestedStart: "2024-01-15T10:00:00-05:00",
			requestedEnd:   "2024-01-16T15:00:00+02:00",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
		{
			name:          "RFC3339Nano format",
			requestedStart: "2024-01-15T10:00:00.123456789Z",
			requestedEnd:   "2024-01-16T10:00:00.987654321Z",
			wantStartZero: false,
			wantEndZero:   false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseDateRange(tt.requestedStart, tt.requestedEnd)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseDateRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if !start.IsZero() && tt.wantStartZero {
					t.Errorf("parseDateRange() start = %v, expected zero time on error", start)
				}
				if !end.IsZero() && tt.wantEndZero {
					t.Errorf("parseDateRange() end = %v, expected zero time on error", end)
				}
				return
			}

			// For successful cases, verify the times are reasonable
			if start.IsZero() {
				t.Error("parseDateRange() start time is zero when it should have a value")
			}
			if end.IsZero() {
				t.Error("parseDateRange() end time is zero when it should have a value")
			}

			// When both are empty, verify defaults are applied correctly
			if tt.requestedStart == "" && tt.requestedEnd == "" {
				// Start should be approximately 24 hours before end
				duration := end.Sub(start)
				expectedDuration := 24 * time.Hour
				tolerance := 2 * time.Second // Allow 2 second tolerance for test execution time

				if duration < expectedDuration-tolerance || duration > expectedDuration+tolerance {
					t.Errorf("parseDateRange() duration = %v, expected approximately %v", duration, expectedDuration)
				}

				// End should be approximately now
				timeSinceEnd := now.Sub(end)
				if timeSinceEnd < -tolerance || timeSinceEnd > tolerance {
					t.Errorf("parseDateRange() end time is %v away from now, expected within %v", timeSinceEnd, tolerance)
				}
			}

			// For specific times, verify exact parsing
			if tt.requestedStart == "2024-01-15T10:00:00Z" && tt.requestedEnd == "2024-01-16T10:00:00Z" {
				expectedStart := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				expectedEnd := time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC)

				if !start.Equal(expectedStart) {
					t.Errorf("parseDateRange() start = %v, expected %v", start, expectedStart)
				}
				if !end.Equal(expectedEnd) {
					t.Errorf("parseDateRange() end = %v, expected %v", end, expectedEnd)
				}
			}
		})
	}
}

func TestParseDateRange_Boundaries(t *testing.T) {
	tests := []struct {
		name           string
		requestedStart string
		requestedEnd   string
		checkFunc      func(t *testing.T, start, end time.Time, err error)
	}{
		{
			name:           "start equals end",
			requestedStart: "2024-01-15T10:00:00Z",
			requestedEnd:   "2024-01-15T10:00:00Z",
			checkFunc: func(t *testing.T, start, end time.Time, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !start.Equal(end) {
					t.Errorf("start %v should equal end %v", start, end)
				}
			},
		},
		{
			name:           "end before start (still valid parse)",
			requestedStart: "2024-01-16T10:00:00Z",
			requestedEnd:   "2024-01-15T10:00:00Z",
			checkFunc: func(t *testing.T, start, end time.Time, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// Function doesn't validate logical order, just parses
				if start.Before(end) {
					t.Errorf("start %v should be after end %v based on input", start, end)
				}
			},
		},
		{
			name:           "very old date",
			requestedStart: "1970-01-01T00:00:00Z",
			requestedEnd:   "2024-01-15T10:00:00Z",
			checkFunc: func(t *testing.T, start, end time.Time, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if start.Year() != 1970 {
					t.Errorf("start year = %v, expected 1970", start.Year())
				}
			},
		},
		{
			name:           "future date",
			requestedStart: "2024-01-15T10:00:00Z",
			requestedEnd:   "2099-12-31T23:59:59Z",
			checkFunc: func(t *testing.T, start, end time.Time, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if end.Year() != 2099 {
					t.Errorf("end year = %v, expected 2099", end.Year())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseDateRange(tt.requestedStart, tt.requestedEnd)
			tt.checkFunc(t, start, end, err)
		})
	}
}
