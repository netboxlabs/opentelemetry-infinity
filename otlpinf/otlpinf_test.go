package otlpinf

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

const (
	TestHost         = "localhost"
	PoliciesApi      = "/api/v1/policies"
	HttpYamlContent  = "application/x-yaml"
	ErrorMessage     = "HTTP status code = %v, wanted %v"
	NewErrorMessage  = "New() error = %v"
	PostErrorMessage = "http.Post() error = %v"
	YamlErrorMessage = "yaml.NewEncoder() error = %v"
)

func TestOtlpInfRestApis(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55680,
	}

	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NewErrorMessage, err)
	}

	otlp.setupRouter()

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/status", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ErrorMessage, w.Code, http.StatusOK)
	}

	// Act
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/capabilities", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ErrorMessage, w.Code, http.StatusOK)
	}

	// Act
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", PoliciesApi, nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ErrorMessage, w.Code, http.StatusOK)
	}

	// Act get invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf(ErrorMessage, w.Code, http.StatusNotFound)
	}

	// Act delete invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf(ErrorMessage, w.Code, http.StatusNotFound)
	}

	// Act invalid header
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", PoliciesApi, nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Errorf(ErrorMessage, w.Code, http.StatusBadRequest)
	}

	// Act invalid policy config
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", PoliciesApi, bytes.NewBuffer([]byte("invalid\n")))
	req.Header.Set("Content-Type", HttpYamlContent)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Errorf(ErrorMessage, w.Code, http.StatusBadRequest)
	}
}

func TestOtlpinfCreateDeletePolicy(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55681,
	}

	server := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp, err := New(logger, &cfg)
	assert.NoError(t, err, NewErrorMessage, err)

	ctx, cancel := context.WithCancel(context.Background())
	err = otlp.Start(ctx, cancel)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	// Act Create Valid Policy
	data := map[string]interface{}{
		policyName: map[string]interface{}{
			"config": map[string]interface{}{
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
		},
	}
	var buf bytes.Buffer
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YamlErrorMessage, err)
	}

	resp, err := http.Post(server+PoliciesApi, HttpYamlContent, &buf)
	if err != nil {
		t.Errorf(PostErrorMessage, err)
	}

	// Assert
	assert.Equal(t, http.StatusCreated, resp.StatusCode, resp)

	// Act Get Policies
	resp, err = http.Get(server + PoliciesApi)
	if err != nil {
		t.Errorf("http.Get() error = %v", err)
	}

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusOK)
	}

	// Act Get Valid Policy
	resp, err = http.Get(server + "/api/v1/policies/" + policyName)
	if err != nil {
		t.Errorf("http.Get() error = %v", err)
	}

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusOK)
	}

	// Act Try to insert same policy
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YamlErrorMessage, err)
	}
	resp, err = http.Post(server+PoliciesApi, HttpYamlContent, &buf)
	if err != nil {
		t.Errorf(PostErrorMessage, err)
	}

	// Assert
	if resp.StatusCode != http.StatusConflict {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusConflict)
	}

	// Act Delete Policy
	req, err := http.NewRequest("DELETE", server+"/api/v1/policies/"+policyName, nil)
	if err != nil {
		t.Errorf("http.NewRequest() error = %v", err)
	}
	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Errorf("client.Do() error = %v", err)
	}

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusOK)
	}

	otlp.Stop(ctx)
}

func TestOtlpinfCreateInvalidPolicy(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55682,
	}

	server := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NewErrorMessage, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = otlp.Start(ctx, cancel)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

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
	if err != nil {
		t.Errorf(YamlErrorMessage, err)
	}

	resp, err := http.Post(server+PoliciesApi, HttpYamlContent, &buf)
	if err != nil {
		t.Errorf(PostErrorMessage, err)
	}

	// Assert
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusForbidden)
	}

	// Act try to insert policy with invalid config
	data[policyName] = map[string]interface{}{
		"config": map[string]interface{}{
			"invalid": nil,
		},
	}
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YamlErrorMessage, err)
	}

	resp, err = http.Post(server+PoliciesApi, HttpYamlContent, &buf)
	if err != nil {
		t.Errorf(PostErrorMessage, err)
	}

	// Assert
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusBadRequest)
	}

	// Act try to insert two policies at once
	data[policyName] = map[string]interface{}{
		"config": map[string]interface{}{
			"invalid": nil,
		},
	}
	data[policyName+"_new"] = map[string]interface{}{
		"config": map[string]interface{}{
			"invalid": nil,
		},
	}
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YamlErrorMessage, err)
	}

	resp, err = http.Post(server+PoliciesApi, HttpYamlContent, &buf)
	if err != nil {
		t.Errorf(PostErrorMessage, err)
	}

	// Assert
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(ErrorMessage, resp.StatusCode, http.StatusBadRequest)
	}

	otlp.Stop(ctx)
}

func TestOtlpinfStartError(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TestHost,
		ServerPort: 55684,
	}

	// Change the temporary directory environment variable to an invalid path
	err := os.Setenv("TMPDIR", "invalid/prefix")
	assert.NoError(t, err)

	// Act and Assert
	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NewErrorMessage, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = otlp.Start(ctx, cancel)

	if err == nil {
		t.Errorf("Expected an error, but got none")
	}
	if !strings.Contains(err.Error(), "invalid/prefix") {
		t.Errorf("Expected an 'invalid/prefix' error, but got: %s", err.Error())
	}

	// Reset the temporary directory environment variable to its original value
	os.Unsetenv("TMPDIR")
}
