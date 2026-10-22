package runner

import (
	"sync"
	"testing"
)

func resetCollector(t *testing.T) {
	t.Helper()
	collectorMu.Lock()
	defer collectorMu.Unlock()
	if collectorExe != nil {
		_ = collectorExe.Close()
	}
	collectorExe, collectorRefs = nil, 0
}

func TestCollectorIsSharedWhileReferenced(t *testing.T) {
	resetCollector(t)
	t.Cleanup(func() { resetCollector(t) })

	first, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	second, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if first != second {
		t.Fatal("expected every holder to share the same collector executable")
	}

	releaseCollector()
	third, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if third != first {
		t.Fatal("expected the collector executable to survive while it is still referenced")
	}

	releaseCollector()
	releaseCollector()
	if collectorExe != nil || collectorRefs != 0 {
		t.Fatalf("expected the executable freed after the last release, refs=%d", collectorRefs)
	}

	fresh, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if fresh == first {
		t.Fatal("expected a new collector executable after the last release")
	}
	releaseCollector()
}

func TestCollectorConcurrentHoldersShareOneExecutable(t *testing.T) {
	resetCollector(t)
	t.Cleanup(func() { resetCollector(t) })

	const holders = 8
	got := make([]any, holders)
	var wg sync.WaitGroup
	for i := range holders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			exe, err := acquireCollector()
			if err != nil {
				t.Errorf(ErrorMessage, err)
				return
			}
			got[i] = exe
		}(i)
	}
	wg.Wait()
	for i := 1; i < holders; i++ {
		if got[i] != got[0] {
			t.Fatalf("holder %d got a different collector executable", i)
		}
	}

	for range holders {
		releaseCollector()
	}
	if collectorExe != nil || collectorRefs != 0 {
		t.Fatalf("expected the executable freed after every holder released, refs=%d", collectorRefs)
	}
}

func TestReleaseCollectorWithoutHoldersIsSafe(t *testing.T) {
	resetCollector(t)
	releaseCollector()
	releaseCollector()
	if collectorRefs != 0 {
		t.Fatalf("expected refs to stay at zero, got %d", collectorRefs)
	}
}
