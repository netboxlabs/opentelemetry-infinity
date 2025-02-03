package runner

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/amenzhinsky/go-memexec"
	"github.com/netboxlabs/opentelemetry-infinity/config"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed otelcol-contrib
var otelContrib []byte

type status int

const (
	unknown status = iota
	running
	runnerError
	offline
)

var mapStatus = map[status]string{
	unknown:     "unknown",
	running:     "running",
	runnerError: "runner_error",
	offline:     "offline",
}

// State represents the state of the runner
type State struct {
	Status        status    `yaml:"-"`
	StatusText    string    `yaml:"status"`
	startTime     time.Time `yaml:"start_time"`
	RestartCount  int64     `yaml:"restart_count"`
	LastLog       string    `yaml:"-"`
	LastError     string    `yaml:"last_error"`
	LastRestartTS time.Time `yaml:"last_restart_time"`
}

// Runner is responsible for executing opentelemetry policies
type Runner struct {
	logger        *zap.Logger
	policyName    string
	policyDir     string
	policyFile    string
	featureGates  string
	sets          []string
	options       []string
	selfTelemetry bool
	state         State
	cancelFunc    context.CancelFunc
	ctx           context.Context
	cmd           *exec.Cmd
	errChan       chan string
}

// GetCapabilities returns the capabilities of the runner
func GetCapabilities(logger *zap.Logger) ([]byte, error) {
	exe, err := memexec.New(otelContrib)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := exe.Close(); err != nil {
			logger.Error("failed to exit", zap.Error(err))
		}
	}()
	cmd := exe.Command("components")
	ret, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// New creates a new runner
func NewRunner(logger *zap.Logger, policyName string, policyDir string, config *config.Config) *Runner {
	return &Runner{
		logger: logger, policyName: policyName, policyDir: policyDir,
		selfTelemetry: config.SelfTelemetry, sets: config.Set, featureGates: config.FeatureGates, errChan: make(chan string),
	}
}

// Configure configures the runner with the given policy
func (r *Runner) Configure(c *config.Policy) error {
	b, err := yaml.Marshal(&c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(r.policyDir, r.policyName)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	r.policyFile = f.Name()
	if err = f.Close(); err != nil {
		return err
	}

	r.options = []string{
		"--config",
		r.policyFile,
	}

	if !r.selfTelemetry {
		r.options = append(r.options, "--set=service.telemetry.metrics.level=None")
	}

	if len(r.featureGates) > 0 {
		r.options = append(r.options, "--feature-gates", r.featureGates)
	}

	if len(r.sets) > 0 {
		r.options = append(r.options, r.sets...)
	}

	return nil
}

// Start starts the runner
func (r *Runner) Start(ctx context.Context, cancelFunc context.CancelFunc) error {
	r.cancelFunc = cancelFunc
	r.ctx = ctx

	exe, err := memexec.New(otelContrib)
	if err != nil {
		return err
	}
	defer func() {
		if err := exe.Close(); err != nil {
			r.logger.Error("failed to exit", zap.Error(err))
		}
	}()

	r.cmd = exe.CommandContext(ctx, r.options...)
	if r.cmd.Err != nil {
		return r.cmd.Err
	}
	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			r.state.LastLog = scanner.Text()
			r.logger.Info("otelcol-contrib", zap.String("policy", r.policyName), zap.String("log", r.state.LastLog))
			if r.cmd.Err != nil {
				r.errChan <- r.state.LastLog
			}
		}
	}()
	if err = r.cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := r.cmd.Wait(); err != nil {
			r.errChan <- r.state.LastLog
			close(r.errChan)
		}
	}()

	reg, _ := regexp.Compile("[^a-zA-Z0-9:(), ]+")

	r.state.startTime = time.Now()
	ctxTimeout, cancel := context.WithTimeout(r.ctx, 1*time.Second)
	defer cancel()
	select {
	case line := <-r.errChan:
		return errors.New(string(append([]byte("otelcol-contrib - "), reg.ReplaceAllString(line, "")...)))
	case <-ctxTimeout.Done():
		r.setStatus(running)
		r.logger.Info("runner proccess started successfully", zap.String("policy", r.policyName), zap.Any("pid", r.cmd.Process.Pid))
	}

	go func() {
		for {
			select {
			case line := <-r.errChan:
				r.state.LastError = string(append([]byte("otelcol-contrib - "), reg.ReplaceAllString(line, "")...))
				r.setStatus(runnerError)
			case <-r.ctx.Done():
				r.Stop(r.ctx)
				return
			}
		}
	}()

	return nil
}

// Stop stops the runner
func (r *Runner) Stop(ctx context.Context) {
	r.logger.Info("routine call to stop runner", zap.Any("routine", ctx.Value("routine")))
	defer r.cancelFunc()
	r.setStatus(offline)
	r.logger.Info("runner process stopped", zap.String("policy", r.policyName))
}

// GetStatus returns the status of the runner
func (r *Runner) GetStatus() State {
	return r.state
}

func (r *Runner) setStatus(s status) {
	r.state.Status = s
	r.state.StatusText = mapStatus[s]
}
