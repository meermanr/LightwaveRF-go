package lwl_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meermanr/LightwaveRF-go/lwl"
)

func TestLatencyStats_String_NoSamples_DoesNotPanic(t *testing.T) {
	ls := lwl.NewLatencyStats("no-samples")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("String() panicked with no samples: %v", r)
		}
	}()

	s := ls.String()
	t.Output().Write([]byte(s))
}

func TestLatencyStats_String_OneSampleA(t *testing.T) {
	ls := lwl.NewLatencyStats("one-sample-a")
	dur := time.Millisecond * 314
	ls.Sample(dur)
	s := ls.String()
	for _, v := range []string{"Min: 314ms", "Max: 314ms", "Mean: 314ms"} {
		if !strings.Contains(s, v) {
			t.Fatal("String() did not include", v, "\n", s)
		}
	}
}

func TestLatencyStats_String_OneSampleB(t *testing.T) {
	ls := lwl.NewLatencyStats("one-sample-b")
	dur := time.Second * 281
	ls.Sample(dur)
	s := ls.String()
	for _, v := range []string{"Min: 4m41s", "Max: 4m41s", "Mean: 4m41s"} {
		if !strings.Contains(s, v) {
			t.Fatal("String() did not include", v, "\n", s)
		}
	}
}

func TestLatencyStats_String_TwoSamplesA(t *testing.T) {
	ls := lwl.NewLatencyStats("two-samples-a")
	dur1 := time.Millisecond * 100
	dur2 := time.Millisecond * 300
	ls.Sample(dur1)
	ls.Sample(dur2)
	s := ls.String()
	for _, v := range []string{"Min: 100ms", "Max: 300ms", "Mean: 200ms"} {
		if !strings.Contains(s, v) {
			t.Fatal("String() did not include", v, "\n", s)
		}
	}
}

func TestLatencyStats_ConcurrentSamples(t *testing.T) {
	ls := lwl.NewLatencyStats("concurrent-samples")

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()
			ls.Sample(time.Millisecond)
		}()
	}

	wg.Wait()

	s := ls.String()
	for _, v := range []string{"Samples: 1000", "Min: 1ms", "Max: 1ms", "Mean: 1ms"} {
		if !strings.Contains(s, v) {
			t.Fatal("String() did not include", v, "\n", s)
		}
	}
}
