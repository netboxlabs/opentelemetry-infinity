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
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v2"
)

const (
	TEST_HOST         = "localhost"
	POLICIES_API      = "/api/v1/policies"
	HTTP_YAML_CONTENT = "application/x-yaml"
	ERROR_MSG         = "HTTP status code = %v, wanted %v"
	NEW_ERR_MSG       = "New() error = %v"
	POST_ERR_MSG      = "http.Post() error = %v"
	YAML_ERR_MSG      = "yaml.NewEncoder() error = %v"
)

func TestOtlpInfRestApis(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TEST_HOST,
		ServerPort: 55680,
	}

	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NEW_ERR_MSG, err)
	}

	otlp.setupRouter()

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/status", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ERROR_MSG, w.Code, http.StatusOK)
	}

	// Act
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/capabilities", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ERROR_MSG, w.Code, http.StatusOK)
	}

	// Act
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", POLICIES_API, nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf(ERROR_MSG, w.Code, http.StatusOK)
	}

	// Act get invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf(ERROR_MSG, w.Code, http.StatusNotFound)
	}

	// Act delete invalid policy
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/policies/invalid_policy", nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusNotFound {
		t.Errorf(ERROR_MSG, w.Code, http.StatusNotFound)
	}

	// Act invalid header
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", POLICIES_API, nil)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Errorf(ERROR_MSG, w.Code, http.StatusBadRequest)
	}

	// Act invalid policy config
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", POLICIES_API, bytes.NewBuffer([]byte("invalid\n")))
	req.Header.Set("Content-Type", HTTP_YAML_CONTENT)
	otlp.router.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Errorf(ERROR_MSG, w.Code, http.StatusBadRequest)
	}
}

func TestOtlpinfCreateDeletePolicy(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TEST_HOST,
		ServerPort: 55681,
	}

	SERVER := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NEW_ERR_MSG, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = otlp.Start(ctx, cancel)

	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	//Act Create Valid Policy
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
		t.Errorf(YAML_ERR_MSG, err)
	}

	resp, err := http.Post(SERVER+POLICIES_API, HTTP_YAML_CONTENT, &buf)
	if err != nil {
		t.Errorf(POST_ERR_MSG, err)
	}

	// Assert
	if resp.StatusCode != http.StatusCreated {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusCreated)
	}

	// Act Get Policies
	resp, err = http.Get(SERVER + POLICIES_API)
	if err != nil {
		t.Errorf("http.Get() error = %v", err)
	}

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusOK)
	}

	// Act Get Valid Policy
	resp, err = http.Get(SERVER + "/api/v1/policies/" + policyName)
	if err != nil {
		t.Errorf("http.Get() error = %v", err)
	}

	// Assert
	if resp.StatusCode != http.StatusOK {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusOK)
	}

	// Act Try to insert same policy
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YAML_ERR_MSG, err)
	}
	resp, err = http.Post(SERVER+POLICIES_API, HTTP_YAML_CONTENT, &buf)
	if err != nil {
		t.Errorf(POST_ERR_MSG, err)
	}

	// Assert
	if resp.StatusCode != http.StatusConflict {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusConflict)
	}

	//Act Delete Policy
	req, err := http.NewRequest("DELETE", SERVER+"/api/v1/policies/"+policyName, nil)
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
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusOK)
	}

	otlp.Stop(ctx)
}

func TestOtlpinfCreateInvalidPolicy(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TEST_HOST,
		ServerPort: 55682,
	}

	SERVER := fmt.Sprintf("http://%s:%v", cfg.ServerHost, cfg.ServerPort)

	// Act and Assert
	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NEW_ERR_MSG, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = otlp.Start(ctx, cancel)

	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	policyName := "policy_test"

	//Act try to insert policy without config
	data := map[string]interface{}{
		policyName: map[string]interface{}{
			"feature_gates": []string{"all"},
		},
	}
	var buf bytes.Buffer
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YAML_ERR_MSG, err)
	}

	resp, err := http.Post(SERVER+POLICIES_API, HTTP_YAML_CONTENT, &buf)
	if err != nil {
		t.Errorf(POST_ERR_MSG, err)
	}

	// Assert
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusForbidden)
	}

	//Act try to insert policy with invalid config
	data[policyName] = map[string]interface{}{
		"config": map[string]interface{}{
			"invalid": nil,
		},
	}
	err = yaml.NewEncoder(&buf).Encode(data)
	if err != nil {
		t.Errorf(YAML_ERR_MSG, err)
	}

	resp, err = http.Post(SERVER+POLICIES_API, HTTP_YAML_CONTENT, &buf)
	if err != nil {
		t.Errorf(POST_ERR_MSG, err)
	}

	// Assert
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusBadRequest)
	}

	//Act try to insert two policies at once
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
		t.Errorf(YAML_ERR_MSG, err)
	}

	resp, err = http.Post(SERVER+POLICIES_API, HTTP_YAML_CONTENT, &buf)
	if err != nil {
		t.Errorf(POST_ERR_MSG, err)
	}

	// Assert
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(ERROR_MSG, resp.StatusCode, http.StatusBadRequest)
	}

	otlp.Stop(ctx)
}

func TestOtlpinfStartError(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	cfg := config.Config{
		Debug:      true,
		ServerHost: TEST_HOST,
		ServerPort: 55684,
	}

	// Change the temporary directory environment variable to an invalid path
	os.Setenv("TMPDIR", "invalid/prefix")

	// Act and Assert
	otlp, err := New(logger, &cfg)
	if err != nil {
		t.Errorf(NEW_ERR_MSG, err)
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
