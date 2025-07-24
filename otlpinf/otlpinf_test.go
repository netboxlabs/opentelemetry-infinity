package otlpinf

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	err := otlp.Start(ctx, cancel)
	assert.NoError(t, err)

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
	var buf bytes.Buffer
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

	otlp.Stop(ctx)
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
	err := otlp.Start(ctx, cancel)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	// Act try to insert policy without config
	data := map[string]interface{}{
		policyName: map[string]interface{}{
			"feature_gates": []string{"all"},
		},
	}
	var buf bytes.Buffer
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

	otlp.Stop(ctx)
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
	err = otlp.Start(ctx, cancel)
	assert.Error(t, err)

	if !strings.Contains(err.Error(), "invalid/prefix") {
		t.Errorf("Expected an 'invalid/prefix' error, but got: %s", err.Error())
	}

	// Reset the temporary directory environment variable to its original value
	err = os.Unsetenv("TMPDIR")
	assert.NoError(t, err)
}
