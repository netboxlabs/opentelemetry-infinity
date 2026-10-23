package runner

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"
)

// The status API reads a runner's state while its goroutines update it.
// Run with -race: an unsynchronized access fails whatever the timing.
func TestRunnerStateIsSafeForConcurrentUse(t *testing.T) {
	r := &Runner{}
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); r.setStatus(running) }()
	go func() { defer wg.Done(); r.recordError("otelcol-contrib - exited") }()
	go func() { defer wg.Done(); r.setLastLog("collector line") }()
	go func() { defer wg.Done(); _ = r.lastLog() }()
	_ = r.GetStatus()
	wg.Wait()

	s := r.GetStatus()
	if s.LastLog != "collector line" || s.LastError != "otelcol-contrib - exited" {
		t.Errorf("got %+v, want both writes kept", s)
	}
}

func TestRunnerRecordError(t *testing.T) {
	r := &Runner{}
	r.setStatus(running)

	r.recordError("otelcol-contrib - exited")

	s := r.GetStatus()
	if s.Status != runnerError || s.StatusText != "runner_error" || s.LastError != "otelcol-contrib - exited" {
		t.Errorf("got %+v, want runner_error with the message", s)
	}
}

// When the collector exits with an error, its watcher sends the last log line
// and closes errChan. The closed channel must not be read as more errors: that
// spun a core and replaced the real error with an empty one.
func TestRunnerMonitorKeepsErrorAfterCollectorExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{logger: slog.New(slog.DiscardHandler), ctx: ctx, cancelFunc: cancel, errChan: make(chan string, 1)}
	done := make(chan struct{})
	go func() { r.monitor(regexp.MustCompile("[^a-zA-Z0-9:(), ]+")); close(done) }()

	r.errChan <- "collector crashed"
	close(r.errChan)
	time.Sleep(50 * time.Millisecond)

	if got := r.GetStatus().LastError; got != "otelcol-contrib - collector crashed" {
		t.Errorf("LastError = %q, want the collector's error", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not return after the context ended")
	}
}
