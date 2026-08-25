package httpx

import (
	"testing"
	"time"
)

func TestTaskUpdatedSince(t *testing.T) {
	cutoff := time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		updatedAt string
		want      bool
	}{
		{name: "after cutoff", updatedAt: "2026-08-24T07:00:01Z", want: true},
		{name: "at cutoff", updatedAt: "2026-08-24T07:00:00Z", want: true},
		{name: "before cutoff", updatedAt: "2026-08-24T06:59:59Z", want: false},
		{name: "invalid timestamp", updatedAt: "unknown", want: false},
		{name: "empty timestamp", updatedAt: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := taskUpdatedSince(test.updatedAt, cutoff); got != test.want {
				t.Fatalf("taskUpdatedSince(%q) = %v, want %v", test.updatedAt, got, test.want)
			}
		})
	}
}
