package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"gopkg.in/yaml.v3"
)

const (
	ErrorMessage = "Expected no error, but got %v"
	TestPolicy   = "test-policy"
)

var PolicyDir = os.TempDir()

func TestRunnerNew(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	c := config.Config{
		SelfTelemetry: false,
	}

	// Act
	runner := NewRunner(logger, TestPolicy, PolicyDir, &c)

	// Assert
	if runner.logger != logger {
		t.Errorf("Expected logger to be set to %v, got %v", logger, runner.logger)
	}

	if runner.policyName != TestPolicy {
		t.Errorf("Expected policyName to be set to %s, got %s", TestPolicy, runner.policyName)
	}

	if runner.policyDir != PolicyDir {
		t.Errorf("Expected policyDir to be set to %s, got %s", PolicyDir, runner.policyDir)
	}

	if runner.selfTelemetry != c.SelfTelemetry {
		t.Errorf("Expected selfTelemetry to be set to %v, got %v", c.SelfTelemetry, runner.selfTelemetry)
	}

	if len(runner.sets) != 0 {
		t.Errorf("Expected sets to be an empty slice, got %v", runner.sets)
	}
}

func TestRunnerConfigure(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	enableTelemetry := false
	runner := &Runner{
		logger:        logger,
		policyName:    TestPolicy,
		policyDir:     PolicyDir,
		selfTelemetry: enableTelemetry,
		featureGates:  "gate1,gate2",
		sets:          []string{"--set=set1=set1"},
	}
	config := &config.Policy{
		Receivers: map[string]interface{}{
			"policy": "value1",
		},
	}

	// Act
	err := runner.Configure(config)
	// Assert
	if err != nil {
		t.Errorf(ErrorMessage, err)
	}

	expectedFeatureGates := "gate1,gate2"
	if !reflect.DeepEqual(runner.featureGates, expectedFeatureGates) {
		t.Errorf("Expected featureGates to be %v, but got %v", expectedFeatureGates, runner.featureGates)
	}

	expectedSet := []string{"--set=set1=set1"}
	if !reflect.DeepEqual(runner.sets, expectedSet) {
		t.Errorf("Expected set to be %v, but got %v", expectedSet, runner.sets)
	}

	if !strings.Contains(runner.policyFile, TestPolicy) {
		t.Errorf("Expected policy File to contain %v, but got %v", TestPolicy, runner.policyFile)
	}
}

func TestRunnerConfigureError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	runner := &Runner{
		logger:        logger,
		policyName:    "invalid/pattern",
		policyDir:     PolicyDir,
		selfTelemetry: true,
	}

	// Error in Yaml Marshal
	policy := &config.Policy{
		Receivers: map[string]interface{}{
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
		t.Errorf(ErrorMessage, err)
	}
	if !strings.Contains(err.Error(), "cannot marshal type: func()") {
		t.Errorf("Expected a 'cannot marshal type: func()' error, but got: %s", err.Error())
	}

	// Error in create temp file
	policy = &config.Policy{
		Receivers: map[string]interface{}{
			"policy": "simple",
		},
	}

	err = runner.Configure(policy)
	if err == nil {
		t.Errorf(ErrorMessage, err)
	}
	if !strings.Contains(err.Error(), "invalid/pattern") {
		t.Errorf("Expected an 'invalid/pattern' error, but got: %s", err.Error())
	}
}

func TestRunnerStartStop(t *testing.T) {
	// Arrange
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))
	runner := &Runner{
		logger:        logger,
		policyName:    TestPolicy,
		policyDir:     PolicyDir,
		selfTelemetry: true,
		featureGates:  "awsemf.nodimrollupdefault,exporter.datadogexporter.DisableAPMStats",
		sets:          []string{"--set=set1=set1", "--set=set2=set2"},
	}
	config := &config.Policy{
		Receivers: map[string]interface{}{
			"policy": "value1",
		},
	}

	// Act
	err := runner.Configure(config)
	if err != nil {
		t.Errorf(ErrorMessage, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = runner.Start(ctx, cancel)
	if err != nil {
		t.Errorf(ErrorMessage, err)
	}

	runner.Stop(ctx)

	s := runner.GetStatus()
	if mapStatus[s.Status] != "offline" {
		t.Errorf("Expected status to be offline, but got %v", mapStatus[s.Status])
	}
}

func TestRunnerGetCapabilities(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: false}))

	// Act
	caps, err := GetCapabilities(logger)
	if err != nil {
		t.Errorf(ErrorMessage, err)
	}

	// Assert
	s := struct {
		Buildinfo struct {
			Version string
		}
	}{}
	err = yaml.Unmarshal(caps, &s)
	if err != nil {
		t.Errorf(ErrorMessage, err)
	}
}
