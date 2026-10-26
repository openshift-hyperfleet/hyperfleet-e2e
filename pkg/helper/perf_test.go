package helper

import (
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
)

// TestAssertMedianBelowThreshold exercises the median/threshold math with
// exact durations, independent of real elapsed time.
func TestAssertMedianBelowThreshold(t *testing.T) {
	tests := []struct {
		name        string
		durations   []time.Duration
		threshold   time.Duration
		wantFailure bool
	}{
		{
			name:      "below-threshold",
			threshold: 50 * time.Millisecond,
			durations: []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond},
		},
		{
			name:        "above-threshold",
			threshold:   50 * time.Millisecond,
			durations:   []time.Duration{60 * time.Millisecond, 70 * time.Millisecond, 80 * time.Millisecond},
			wantFailure: true,
		},
		{
			// Strict '<': an exact match must still fail.
			name:        "equals-threshold",
			threshold:   50 * time.Millisecond,
			durations:   []time.Duration{50 * time.Millisecond},
			wantFailure: true,
		},
		{
			// 100ms outlier among four 1ms samples: mean (~20.8ms) exceeds
			// the threshold, median (1ms) doesn't.
			name:      "median-not-mean",
			threshold: 15 * time.Millisecond,
			durations: []time.Duration{
				1 * time.Millisecond, 1 * time.Millisecond, 100 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond,
			},
		},
		{
			// 10/20/40/50ms averages to 30ms; lower-middle (20ms) would
			// wrongly pass this threshold.
			name:      "even-sample-count-lower-middle",
			threshold: 25 * time.Millisecond,
			durations: []time.Duration{
				10 * time.Millisecond,
				20 * time.Millisecond,
				40 * time.Millisecond,
				50 * time.Millisecond,
			},
			wantFailure: true,
		},
		{
			// 10/20/40/50ms averages to 30ms; upper-middle (40ms) would
			// wrongly fail this threshold.
			name:      "even-sample-count-upper-middle",
			threshold: 35 * time.Millisecond,
			durations: []time.Duration{
				10 * time.Millisecond,
				20 * time.Millisecond,
				40 * time.Millisecond,
				50 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := gomega.InterceptGomegaFailure(func() {
				assertMedianBelowThreshold(tt.name, tt.threshold, tt.durations)
			})

			if tt.wantFailure && failure == nil {
				t.Fatal("expected a failure, got none")
			}
			if !tt.wantFailure && failure != nil {
				t.Fatalf("expected no failure, got: %v", failure)
			}
		})
	}
}

// TestMeasureMedianLatency exercises the real-timing wrapper: call count,
// index passing, and non-positive n rejection. The threshold is generous
// since no case here depends on the median/threshold comparison itself.
func TestMeasureMedianLatency(t *testing.T) {
	t.Run("calls sampleFn exactly n times with its index", func(t *testing.T) {
		const n = 7
		var gotIndexes []int

		failure := gomega.InterceptGomegaFailure(func() {
			MeasureMedianLatency("sample-n-times", time.Second, n, func(i int) {
				gotIndexes = append(gotIndexes, i)
			})
		})

		if failure != nil {
			t.Fatalf("unexpected failure: %v", failure)
		}
		if len(gotIndexes) != n {
			t.Fatalf("expected sampleFn to be called %d times, got %d", n, len(gotIndexes))
		}
		for i, got := range gotIndexes {
			if got != i {
				t.Fatalf("expected call %d to receive index %d, got %d", i, i, got)
			}
		}
	})

	t.Run("rejects non-positive sample counts", func(t *testing.T) {
		for _, n := range []int{0, -1} {
			t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
				called := false
				failure := gomega.InterceptGomegaFailure(func() {
					MeasureMedianLatency("invalid-n", 50*time.Millisecond, n, func(int) { called = true })
				})

				if failure == nil {
					t.Fatalf("expected a validation failure for n=%d, got none", n)
				}
				if called {
					t.Fatalf("expected sampleFn not to be called for n=%d", n)
				}
			})
		}
	})
}
