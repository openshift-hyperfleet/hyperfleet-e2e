package helper

import (
	"slices"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// init registers a fail handler so gomega.Expect works under
// `go test ./pkg/helper/...`, outside the e2e binary's own registration in
// pkg/e2e/e2e.go. Double registration is harmless.
func init() {
	gomega.RegisterFailHandler(ginkgo.Fail)
}

// DefaultSamples is the sample count for perf specs measuring a single,
// self-contained operation.
const DefaultSamples = 5

// MeasureMedianLatency calls sampleFn n times with its index, records each
// call's duration, and asserts the median is below threshold.
//
// Callers are ginkgo.Serial, so no other spec runs while this one samples.
// The HTTP connection pool may still be cold for the first sample: in
// perf-nightly, process 1 may not have run a parallel spec before the Serial
// ones. The median absorbs that one slow sample. Parallel callers would need a
// per-process warmup in SynchronizedBeforeSuite.
func MeasureMedianLatency(name string, threshold time.Duration, n int, sampleFn func(i int)) {
	gomega.Expect(n).To(gomega.BeNumerically(">", 0), "MeasureMedianLatency: sample count must be positive, got %d", n)

	durations := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		sampleFn(i)
		durations[i] = time.Since(start)
	}

	assertMedianBelowThreshold(name, threshold, durations)
}

// assertMedianBelowThreshold computes the median of durations (averaging the
// two middle values for an even count) and asserts it's below threshold.
// durations must be non-empty; callers are responsible for that (see the
// n > 0 check in MeasureMedianLatency).
func assertMedianBelowThreshold(name string, threshold time.Duration, durations []time.Duration) {
	slices.Sort(durations)

	n := len(durations)
	median := durations[n/2]
	if n%2 == 0 {
		median = (durations[n/2-1] + durations[n/2]) / 2
	}
	ginkgo.GinkgoWriter.Printf(
		"[PERF] [proc %d] %s: median=%v (n=%d)\n",
		ginkgo.GinkgoParallelProcess(),
		name,
		median,
		n,
	)

	gomega.Expect(median).To(gomega.BeNumerically("<", threshold),
		"%s exceeded threshold: median=%v threshold=%v", name, median, threshold)
}
