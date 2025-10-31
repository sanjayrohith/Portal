package proxy

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// LatencySummary reports throughput and percentile latencies for a burst run.
type LatencySummary struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	RPS   float64
}

func (s LatencySummary) String() string {
	return fmt.Sprintf("n=%d min=%s p50=%s p95=%s p99=%s max=%s mean=%s rps=%.1f",
		s.Count, s.Min, s.P50, s.P95, s.P99, s.Max, s.Mean, s.RPS)
}

// LatencyRecorder collects per-request latencies concurrently and computes
// percentile statistics. Use RunBurst to drive a concurrent request burst.
type LatencyRecorder struct {
	mu      sync.Mutex
	samples []time.Duration
	elapsed time.Duration
}

// Record appends a single request latency.
func (r *LatencyRecorder) Record(d time.Duration) {
	r.mu.Lock()
	r.samples = append(r.samples, d)
	r.mu.Unlock()
}

// percentile returns the p-th percentile (p in [0,100]) via nearest-rank.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int((p / 100) * float64(len(sorted)))
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// Summary computes count/min/max/mean/p50/p95/p99 over recorded samples.
func (r *LatencyRecorder) Summary() LatencySummary {
	r.mu.Lock()
	samples := append([]time.Duration(nil), r.samples...)
	elapsed := r.elapsed
	r.mu.Unlock()

	summary := LatencySummary{Count: len(samples)}
	if len(samples) == 0 {
		return summary
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	summary.Min = samples[0]
	summary.Max = samples[len(samples)-1]
	var total time.Duration
	for _, s := range samples {
		total += s
	}
	summary.Mean = total / time.Duration(len(samples))
	summary.P50 = percentile(samples, 50)
	summary.P95 = percentile(samples, 95)
	summary.P99 = percentile(samples, 99)
	if elapsed > 0 {
		summary.RPS = float64(len(samples)) / elapsed.Seconds()
	}
	return summary
}

// RunBurst drives fn concurrently across workers*wps invocations, recording
// each invocation latency. It returns the summary of the burst.
func RunBurst(workers, perWorker int, fn func(worker, iter int) error) (LatencySummary, error) {
	recorder := &LatencyRecorder{}
	var wg sync.WaitGroup
	errCh := make(chan error, workers*perWorker)
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				begin := time.Now()
				if err := fn(worker, i); err != nil {
					errCh <- err
					return
				}
				recorder.Record(time.Since(begin))
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	recorder.mu.Lock()
	recorder.elapsed = time.Since(start)
	recorder.mu.Unlock()
	for err := range errCh {
		if err != nil {
			return recorder.Summary(), err
		}
	}
	return recorder.Summary(), nil
}
