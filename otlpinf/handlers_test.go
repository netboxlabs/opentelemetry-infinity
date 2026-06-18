package otlpinf

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/netboxlabs/opentelemetry-infinity/config"
)

func newTestOtlp() *OltpInf {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	o := NewOtlp(logger, &config.Config{ServerHost: TestHost})
	o.setupRouter()
	return o
}

// getCapabilities returns 400 when the stored capabilities are not valid YAML.
func TestGetCapabilitiesError(t *testing.T) {
	o := newTestOtlp()
	o.capabilities = []byte("{") // invalid YAML -> yson.YAMLToJSON fails

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/capabilities", nil)
	o.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// createPolicy returns 409 when the policy already exists (no collector started).
func TestCreatePolicyConflict(t *testing.T) {
	o := newTestOtlp()
	o.policies["existing"] = RunnerInfo{}

	body := "existing:\n  receivers:\n    otlp:\n  exporters:\n    debug:\n  service: {}\n"
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", PoliciesAPI, strings.NewReader(body))
	req.Header.Set("Content-Type", HTTPYamlContent)
	o.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// createPolicy returns 400 when Configure fails (invalid policies dir), before any exec.
func TestCreatePolicyConfigureError(t *testing.T) {
	o := newTestOtlp()
	o.policiesDir = "/nonexistent/policies/dir"

	body := "p1:\n  receivers:\n    otlp:\n  exporters:\n    debug:\n  service: {}\n"
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", PoliciesAPI, strings.NewReader(body))
	req.Header.Set("Content-Type", HTTPYamlContent)
	o.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// Stop with no http server removes the policies dir and clears state.
func TestStopRemovesPoliciesDir(t *testing.T) {
	o := newTestOtlp()
	dir := t.TempDir()
	o.policiesDir = dir
	o.cancelFunction = func() {}

	o.Stop(context.Background())

	if o.policiesDir != "" {
		t.Errorf("expected policiesDir cleared, got %q", o.policiesDir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected dir %q removed, stat err = %v", dir, err)
	}
}

// Stop shuts down and clears a non-nil http server.
func TestStopShutsDownServer(t *testing.T) {
	o := newTestOtlp()
	o.httpServer = &http.Server{Addr: "127.0.0.1:0"}
	o.cancelFunction = func() {}

	o.Stop(context.Background())

	if o.httpServer != nil {
		t.Errorf("expected httpServer cleared")
	}
}

// startFailure delivers the error and cleans up an existing policies dir.
func TestStartFailureCleansPoliciesDir(t *testing.T) {
	o := newTestOtlp()
	dir := t.TempDir()
	o.policiesDir = dir

	ch := o.startFailure(errors.New("boom"))
	err, ok := <-ch
	if !ok || err == nil {
		t.Fatalf("expected an error on the channel, ok=%v err=%v", ok, err)
	}
	if _, more := <-ch; more {
		t.Errorf("expected channel closed after one error")
	}
	if o.policiesDir != "" {
		t.Errorf("expected policiesDir cleared, got %q", o.policiesDir)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("expected dir %q removed", dir)
	}
}
