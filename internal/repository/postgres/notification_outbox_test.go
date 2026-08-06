package postgres

import (
	"testing"
	"time"
)

func TestExponentialBackoffIsCapped(t *testing.T) {
	base := time.Second
	maximum := 8 * time.Second
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 4, want: 8 * time.Second},
		{attempt: 100, want: 8 * time.Second},
	}
	for _, test := range tests {
		if got := exponentialBackoff(test.attempt, base, maximum); got != test.want {
			t.Fatalf("attempt %d: got %s, want %s", test.attempt, got, test.want)
		}
	}
}
