package runner

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v2"
)

const (
	ERROR_MSG   = "Expected no error, but got %v"
	TEST_POLICY = "test-policy"
)

var POLICY_DIR = os.TempDir()

func TestRunnerNew(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	selfTelemetry := false

	// Act
	runner := New(logger, TEST_POLICY, POLICY_DIR, selfTelemetry)

	// Assert
	if runner.logger != logger {
		t.Errorf("Expected logger to be set to %v, got %v", logger, runner.logger)
	}

	if runner.policyName != TEST_POLICY {
		t.Errorf("Expected policyName to be set to %s, got %s", TEST_POLICY, runner.policyName)
	}

	if runner.policyDir != POLICY_DIR {
		t.Errorf("Expected policyDir to be set to %s, got %s", POLICY_DIR, runner.policyDir)
	}

	if runner.selfTelemetry != selfTelemetry {
		t.Errorf("Expected selfTelemetry to be set to %v, got %v", selfTelemetry, runner.selfTelemetry)
	}

	if len(runner.sets) != 0 {
		t.Errorf("Expected sets to be an empty slice, got %v", runner.sets)
	}
}

func TestRunnerConfigure(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	enableTelemetry := false
	runner := &Runner{
		logger:        logger,
		policyName:    TEST_POLICY,
		policyDir:     POLICY_DIR,
		selfTelemetry: enableTelemetry,
	}
	config := &config.Policy{
		FeatureGates: []string{"gate1", "gate2"},
		Set: map[string]string{
			"set1": "set1",
		},
		Config: map[string]interface{}{
			"policy": "value1",
		},
	}

	// Act
	err := runner.Configure(config)

	// Assert
	if err != nil {
		t.Errorf(ERROR_MSG, err)
	}

	expectedFeatureGates := "gate1,gate2"
	if !reflect.DeepEqual(runner.featureGates, expectedFeatureGates) {
		t.Errorf("Expected featureGates to be %v, but got %v", expectedFeatureGates, runner.featureGates)
	}

	expectedSet := []string{"--set=set1=set1"}
	if !reflect.DeepEqual(runner.sets, expectedSet) {
		t.Errorf("Expected set to be %v, but got %v", expectedSet, runner.sets)
	}

	if !strings.Contains(runner.policyFile, TEST_POLICY) {
		t.Errorf("Expected policy File to contain %v, but got %v", TEST_POLICY, runner.policyFile)
	}
}

func TestRunnerConfigureError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	runner := &Runner{
		logger:        logger,
		policyName:    "invalid/pattern",
		policyDir:     POLICY_DIR,
		selfTelemetry: true,
	}

	// Error in Yaml Marshal
	policy := &config.Policy{
		Config: map[string]interface{}{
			"function": func() {},
		},
	}

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("Recovered from panic: %v", r)
			}
		}()
		err = runner.Configure(policy)
	}()
	if err == nil {
		t.Errorf(ERROR_MSG, err)
	}
	if !strings.Contains(err.Error(), "cannot marshal type: func()") {
		t.Errorf("Expected a 'cannot marshal type: func()' error, but got: %s", err.Error())
	}

	//Error in create temp file
	policy = &config.Policy{
		Config: map[string]interface{}{
			"policy": "simple",
		},
	}

	err = runner.Configure(policy)
	if err == nil {
		t.Errorf(ERROR_MSG, err)
	}
	if !strings.Contains(err.Error(), "invalid/pattern") {
		t.Errorf("Expected an 'invalid/pattern' error, but got: %s", err.Error())
	}
}

func TestRunnerStartStop(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)
	runner := &Runner{
		logger:        logger,
		policyName:    TEST_POLICY,
		policyDir:     POLICY_DIR,
		selfTelemetry: true,
	}
	config := &config.Policy{
		FeatureGates: []string{"awsemf.nodimrollupdefault", "exporter.datadogexporter.DisableAPMStats"},
		Set: map[string]string{
			"set1": "set1",
			"set2": "set2",
		},
		Config: map[string]interface{}{
			"policy": "value1",
		},
	}

	//Act
	err := runner.Configure(config)
	if err != nil {
		t.Errorf(ERROR_MSG, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = runner.Start(ctx, cancel)
	if err != nil {
		t.Errorf(ERROR_MSG, err)
	}

	runner.Stop(ctx)

	s := runner.GetStatus()
	if MapStatus[s.Status] != "offline" {
		t.Errorf("Expected status to be offline, but got %v", MapStatus[s.Status])
	}
}

func TestRunnerGetCapabilities(t *testing.T) {
	//Act
	caps, err := GetCapabilities()
	if err != nil {
		t.Errorf(ERROR_MSG, err)
	}

	// Assert
	s := struct {
		Buildinfo struct {
			Version string
		}
	}{}
	err = yaml.Unmarshal(caps, &s)
	if err != nil {
		t.Errorf(ERROR_MSG, err)
	}
}
