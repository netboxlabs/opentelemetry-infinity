package otlpinf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netboxlabs/opentelemetry-infinity/config"
)

const (
	TestHost        = "localhost"
	PoliciesAPI     = "/api/v1/policies"
	HTTPYamlContent = "application/x-yaml"
)

func TestOtlpInfRestApis(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55680,
	}

	otlp := NewOtlp(logger, &cfg)
	otlp.setupRouter()

	// Act and Assert
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/status", nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Act and Assert
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/capabilities", nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Act and Assert
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", PoliciesAPI, nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Act and Assert get invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Act and Asset delete invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Act and Assert invalid header
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", PoliciesAPI, nil)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Act and Assert invalid policy config
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", PoliciesAPI, bytes.NewBuffer([]byte("invalid\n")))
	req.Header.Set("Content-Type", HTTPYamlContent)
	otlp.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOtlpinfCreateDeletePolicy(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55681,
	}

	server := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp := NewOtlp(logger, &cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverErrCh := otlp.Start(ctx, cancel)
	require.NotNil(t, serverErrCh)

	select {
	case err, ok := <-serverErrCh:
		if ok && err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
	default:
	}

	t.Cleanup(func() {
		otlp.Stop(ctx)
		for err := range serverErrCh {
			if err != nil {
				t.Logf("server returned error: %v", err)
			}
		}
	})

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	// Act Create Valid Policy
	data := map[string]interface{}{
		policyName: map[string]interface{}{
			"receivers": map[string]interface{}{
				"otlp": map[string]interface{}{
					"protocols": map[string]interface{}{
						"http": nil,
						"grpc": nil,
					},
				},
			},
			"exporters": map[string]interface{}{
				"debug": map[string]interface{}{},
			},
			"service": map[string]interface{}{
				"pipelines": map[string]interface{}{
					"metrics": map[string]interface{}{
						"receivers": []string{"otlp"},
						"exporters": []string{"debug"},
					},
				},
			},
		},
	}
	var (
		buf bytes.Buffer
		err error
	)
	err = yaml.NewEncoder(&buf).Encode(data)
	assert.NoError(t, err)

	resp, err := http.Post(server+PoliciesAPI, HTTPYamlContent, &buf)
	assert.NoError(t, err)
	err = resp.Body.Close()
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusCreated, resp.StatusCode, resp)

	// Act and Assert Get Policies
	resp, err = http.Get(server + PoliciesAPI)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)

	// Act and Assert Get Valid Policy
	resp, err = http.Get(server + "/api/v1/policies/" + policyName)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)

	// Act Try to insert same policy
	err = yaml.NewEncoder(&buf).Encode(data)
	assert.NoError(t, err)

	resp, err = http.Post(server+PoliciesAPI, HTTPYamlContent, &buf)
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)
	// Act Delete Policy
	req, err := http.NewRequest("DELETE", server+"/api/v1/policies/"+policyName, nil)
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err = client.Do(req)
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)
}

func TestOtlpinfCreateInvalidPolicy(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55682,
	}

	server := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp := NewOtlp(logger, &cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverErrCh := otlp.Start(ctx, cancel)
	require.NotNil(t, serverErrCh)

	select {
	case err, ok := <-serverErrCh:
		if ok && err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
	default:
	}

	t.Cleanup(func() {
		otlp.Stop(ctx)
		for err := range serverErrCh {
			if err != nil {
				t.Logf("server returned error: %v", err)
			}
		}
	})

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	// Act try to insert policy without config
	data := map[string]interface{}{
		policyName: map[string]interface{}{
			"feature_gates": []string{"all"},
		},
	}
	var (
		buf bytes.Buffer
		err error
	)
	err = yaml.NewEncoder(&buf).Encode(data)
	assert.NoError(t, err)

	resp, err := http.Post(server+PoliciesAPI, HTTPYamlContent, &buf)
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)

	// Act try to insert policy with invalid config
	data[policyName] = map[string]interface{}{
		"receivers": map[string]interface{}{
			"invalid": nil,
		},
	}
	err = yaml.NewEncoder(&buf).Encode(data)
	assert.NoError(t, err)

	resp, err = http.Post(server+PoliciesAPI, HTTPYamlContent, &buf)
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)

	// Act try to insert two policies at once
	data[policyName] = map[string]interface{}{
		"receivers": map[string]interface{}{
			"invalid": nil,
		},
	}
	data[policyName+"_new"] = map[string]interface{}{
		"receivers": map[string]interface{}{
			"invalid": nil,
		},
	}
	err = yaml.NewEncoder(&buf).Encode(data)
	assert.NoError(t, err)

	resp, err = http.Post(server+PoliciesAPI, HTTPYamlContent, &buf)
	assert.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	err = resp.Body.Close()
	assert.NoError(t, err)
}

// startTestServer starts otlpinf on port and returns its base URL.
func startTestServer(t *testing.T, port uint64) string {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	cfg := config.Config{Debug: true, ServerHost: TestHost, ServerPort: port}
	otlp := NewOtlp(logger, &cfg)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverErrCh := otlp.Start(ctx, cancel)
	require.NotNil(t, serverErrCh)
	t.Cleanup(func() {
		otlp.Stop(ctx)
		for err := range serverErrCh {
			if err != nil {
				t.Logf("server returned error: %v", err)
			}
		}
	})
	time.Sleep(100 * time.Millisecond)
	select {
	case err, ok := <-serverErrCh:
		if ok && err != nil {
			t.Fatalf("failed to start server: %v", err)
		}
	default:
	}
	return fmt.Sprintf("http://%s:%v", TestHost, port)
}

// otlpPolicy listens for OTLP over gRPC on addr.
func otlpPolicy(addr string) map[string]interface{} {
	return map[string]interface{}{
		"receivers": map[string]interface{}{
			"otlp": map[string]interface{}{
				"protocols": map[string]interface{}{
					"grpc": map[string]interface{}{"endpoint": addr},
				},
			},
		},
		"exporters": map[string]interface{}{"debug": map[string]interface{}{}},
		"service": map[string]interface{}{
			"pipelines": map[string]interface{}{
				"metrics": map[string]interface{}{
					"receivers": []string{"otlp"},
					"exporters": []string{"debug"},
				},
			},
		},
	}
}

// portFree reports whether addr can be listened on, retrying until timeout:
// a stopped collector exits a moment after the request returns.
func portFree(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if l, err := net.Listen("tcp", addr); err == nil {
			_ = l.Close()
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// hostmetricsPolicy binds no port, so several copies can run at once.
func hostmetricsPolicy() map[string]interface{} {
	return map[string]interface{}{
		"receivers": map[string]interface{}{
			"hostmetrics": map[string]interface{}{
				"collection_interval": "60s",
				"scrapers":            map[string]interface{}{"load": nil},
			},
		},
		"exporters": map[string]interface{}{"debug": map[string]interface{}{}},
		"service": map[string]interface{}{
			"pipelines": map[string]interface{}{
				"metrics": map[string]interface{}{
					"receivers": []string{"hostmetrics"},
					"exporters": []string{"debug"},
				},
			},
		},
	}
}

// sendPolicyRequest sends body, if any, as YAML and returns the status code and response body.
func sendPolicyRequest(method, url string, body map[string]interface{}) (int, string, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := yaml.NewEncoder(&buf).Encode(body); err != nil {
			return 0, "", err
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", HTTPYamlContent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), err
}

// A request whose second policy fails stops the first and leaves neither behind.
func TestOtlpinfCreatePoliciesRollsBack(t *testing.T) {
	server := startTestServer(t, 55683)
	do := func(method, path string, body map[string]interface{}) (int, string) {
		t.Helper()
		code, resp, err := sendPolicyRequest(method, server+path, body)
		require.NoError(t, err)
		return code, resp
	}
	const validAddr = "127.0.0.1:55687"
	invalid := map[string]interface{}{
		"receivers": map[string]interface{}{"invalid": nil},
	}

	// The valid policy starts on its own and holds its port while it runs.
	code, body := do("POST", PoliciesAPI, map[string]interface{}{"a_valid": otlpPolicy(validAddr)})
	require.Equal(t, http.StatusCreated, code, body)
	require.Eventually(t, func() bool { return !portFree(validAddr, 0) }, 5*time.Second, 50*time.Millisecond,
		"a running a_valid holds %s", validAddr)
	code, body = do("DELETE", PoliciesAPI+"/a_valid", nil)
	require.Equal(t, http.StatusOK, code, body)
	require.True(t, portFree(validAddr, 5*time.Second), "a deleted a_valid frees %s", validAddr)

	code, body = do("POST", PoliciesAPI, map[string]interface{}{"a_valid": otlpPolicy(validAddr), "b_invalid": invalid})
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.True(t, portFree(validAddr, 5*time.Second), "the rollback stops a_valid and frees %s", validAddr)

	code, body = do("GET", PoliciesAPI, nil)
	assert.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, "[]", body)
	code, _ = do("GET", PoliciesAPI+"/a_valid", nil)
	assert.Equal(t, http.StatusNotFound, code)
}

// Create and delete requests run alongside requests that list the policies.
func TestOtlpinfPolicyRequestsConcurrently(t *testing.T) {
	server := startTestServer(t, 55686)

	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 2 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				if code, body, err := sendPolicyRequest("GET", server+PoliciesAPI, nil); err != nil || code != http.StatusOK {
					t.Errorf("GET: %d %v %s", code, err, body)
					return
				}
			}
		})
	}

	var writers sync.WaitGroup
	for w := range 2 {
		writers.Go(func() {
			for i := range 2 {
				name := fmt.Sprintf("w%d_%d", w, i)
				if code, body, err := sendPolicyRequest("POST", server+PoliciesAPI, map[string]interface{}{name: hostmetricsPolicy()}); err != nil || code != http.StatusCreated {
					t.Errorf("POST %s: %d %v %s", name, code, err, body)
					return
				}
				if code, body, err := sendPolicyRequest("DELETE", server+PoliciesAPI+"/"+name, nil); err != nil || code != http.StatusOK {
					t.Errorf("DELETE %s: %d %v %s", name, code, err, body)
					return
				}
			}
		})
	}
	writers.Wait()
	close(done)
	readers.Wait()
}

// Requests that create the same policy at once start it only once.
func TestOtlpinfCreateSamePolicyConcurrently(t *testing.T) {
	server := startTestServer(t, 55685)

	const requests = 4
	codes := make([]int, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() {
			code, body, err := sendPolicyRequest("POST", server+PoliciesAPI, map[string]interface{}{"same": hostmetricsPolicy()})
			if err != nil {
				t.Errorf("POST: %v", err)
				return
			}
			if code != http.StatusCreated && code != http.StatusConflict {
				t.Errorf("POST: unexpected status %d: %s", code, body)
			}
			codes[i] = code
		})
	}
	wg.Wait()

	created := 0
	for _, code := range codes {
		if code == http.StatusCreated {
			created++
		}
	}
	assert.Equal(t, 1, created, "status codes: %v", codes)
}

func TestOtlpinfStartError(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55684,
	}

	// Change the temporary directory environment variable to an invalid path
	err := os.Setenv("TMPDIR", "invalid/prefix")
	assert.NoError(t, err)

	// Act and Assert
	otlp := NewOtlp(logger, &cfg)

	ctx, cancel := context.WithCancel(context.Background())
	serverErrCh := otlp.Start(ctx, cancel)
	require.NotNil(t, serverErrCh)

	err, ok := <-serverErrCh
	require.True(t, ok)
	assert.Error(t, err)

	_, ok = <-serverErrCh
	assert.False(t, ok)

	if !strings.Contains(err.Error(), "invalid/prefix") {
		t.Errorf("Expected an 'invalid/prefix' error, but got: %s", err.Error())
	}

	// Reset the temporary directory environment variable to its original value
	err = os.Unsetenv("TMPDIR")
	assert.NoError(t, err)
}
