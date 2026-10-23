package runner

import (
	"sync"
	"testing"
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
