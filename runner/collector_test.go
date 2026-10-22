package runner

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/amenzhinsky/go-memexec"

	"github.com/netboxlabs/opentelemetry-infinity/config"
)

func collectorRefCount() int {
	collectorMu.Lock()
	defer collectorMu.Unlock()
	return collectorRefs
}

// waitForNoCollectorHolders waits for collectors started by earlier tests to
// exit and give back their references, so each test starts from zero.
func waitForNoCollectorHolders(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for collectorRefCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("collector still has %d holders", collectorRefCount())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertFreed(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the collector copy at %s to be freed, stat error: %v", path, err)
	}
}

func TestCollectorIsSharedWhileReferenced(t *testing.T) {
	waitForNoCollectorHolders(t)

	first, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	path := first.Command().Path
	second, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if first != second {
		t.Fatal("expected every holder to share the same collector executable")
	}

	if err := releaseCollector(); err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the copy to survive while still referenced: %v", err)
	}

	if err := releaseCollector(); err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	assertFreed(t, path)

	fresh, err := acquireCollector()
	if err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if fresh == first {
		t.Fatal("expected a new collector executable after the last release")
	}
	if err := releaseCollector(); err != nil {
		t.Fatalf(ErrorMessage, err)
	}
}

func TestCollectorConcurrentHoldersShareOneExecutable(t *testing.T) {
	waitForNoCollectorHolders(t)

	const holders = 8
	got := make([]*memexec.Exec, holders)
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
	path := got[0].Command().Path
	for range holders {
		if err := releaseCollector(); err != nil {
			t.Fatalf(ErrorMessage, err)
		}
	}
	assertFreed(t, path)
}

func TestReleaseCollectorWithoutHolderReportsIt(t *testing.T) {
	waitForNoCollectorHolders(t)
	if err := releaseCollector(); !errors.Is(err, errNoCollectorHolder) {
		t.Fatalf("expected errNoCollectorHolder, got %v", err)
	}
	if n := collectorRefCount(); n != 0 {
		t.Fatalf("expected no holders, got %d", n)
	}
}

func newTestRunner(t *testing.T, policy *config.Policy) *Runner {
	t.Helper()
	r := &Runner{
		logger:     slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		policyName: TestPolicy,
		policyDir:  PolicyDir,
		errChan:    make(chan string),
	}
	if err := r.Configure(policy); err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	return r
}

// The embedded collector only runs on Linux; elsewhere Start fails before the
// process exists.
func skipUnlessCollectorRuns(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the embedded collector is a Linux binary")
	}
}

var invalidPolicy = &config.Policy{Receivers: map[string]interface{}{"policy": "value1"}}

var nopPolicy = &config.Policy{
	Receivers: map[string]interface{}{"nop": map[string]interface{}{}},
	Exporters: map[string]interface{}{"nop": map[string]interface{}{}},
	Service: map[string]interface{}{
		"pipelines": map[string]interface{}{
			"metrics": map[string]interface{}{"receivers": []string{"nop"}, "exporters": []string{"nop"}},
		},
	},
}

func TestRunnerStartReleasesCollectorWhenProcessNeverStarts(t *testing.T) {
	waitForNoCollectorHolders(t)
	r := newTestRunner(t, invalidPolicy)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Start(ctx, cancel); err == nil {
		t.Fatal("expected Start to fail with a cancelled context")
	}
	if n := collectorRefCount(); n != 0 {
		t.Fatalf("expected the reference given back, got %d holders", n)
	}
}

func TestRunnerReleasesCollectorWhenProcessExits(t *testing.T) {
	skipUnlessCollectorRuns(t)
	waitForNoCollectorHolders(t)
	r := newTestRunner(t, invalidPolicy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A slow runner can outlast Start's one-second window, and the exit is then
	// reported later; either way the reference must come back.
	if err := r.Start(ctx, cancel); err == nil {
		t.Log("the collector exited after Start returned")
	}
	waitForNoCollectorHolders(t)
}

func TestRunnerReleasesCollectorWhenStopped(t *testing.T) {
	skipUnlessCollectorRuns(t)
	waitForNoCollectorHolders(t)
	r := newTestRunner(t, nopPolicy)

	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Start(ctx, cancel); err != nil {
		t.Fatalf(ErrorMessage, err)
	}
	if n := collectorRefCount(); n != 1 {
		t.Fatalf("expected the running collector to hold the copy, got %d holders", n)
	}
	r.Stop(ctx)
	waitForNoCollectorHolders(t)
}
