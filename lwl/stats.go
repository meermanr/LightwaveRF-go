package lwl

import (
	"fmt"
	"sync"
	"time"
)

// LatencyStats maintains statistics (min/mean/max duration)
type LatencyStats struct {
	mu    sync.RWMutex
	name  string // Identify to print in .String()
	count int64  // Number of samples collected
	total time.Duration
	min   time.Duration
	max   time.Duration
}

// NewLatencyStats returns a *LatencyStats
//
// Returns a pointer-owned struct to prevent its mutex getting copied when
// passed around (e.g. stored in a map)
func NewLatencyStats(name string) *LatencyStats {
	return &LatencyStats{name: name}
}

// Sample updates counts and matrics with the seen duration
func (l *LatencyStats) Sample(t time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.count++
	l.total += t
	if l.min == 0 || l.min > t {
		l.min = t
	}
	if t > l.max {
		l.max = t
	}
}

func (l *LatencyStats) String() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var mean time.Duration // Rely on zero-value if no samples
	if l.count > 0 {
		mean = time.Duration(l.total.Nanoseconds() / l.count)
	}
	return fmt.Sprintf(
		`
%s:
  Samples: %v
      Max: %v
     Mean: %v
      Min: %v
`,
		l.name,
		l.count,
		l.max,
		mean,
		l.min,
	)
}
