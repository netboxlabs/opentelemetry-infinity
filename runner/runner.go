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
var otel_contrib []byte

type Status int

const (
	Unknown Status = iota
	Running
	RunnerError
	Offline
)

var MapStatus = map[Status]string{
	Unknown:     "unknown",
	Running:     "running",
	RunnerError: "runner_error",
	Offline:     "offline",
}

type State struct {
	Status        Status    `yaml:"-"`
	StatusText    string    `yaml:"status"`
	startTime     time.Time `yaml:"start_time"`
	RestartCount  int64     `yaml:"restart_count"`
	LastLog       string    `yaml:"-"`
	LastError     string    `yaml:"last_error"`
	LastRestartTS time.Time `yaml:"last_restart_time"`
}

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

func GetCapabilities(logger *zap.Logger) ([]byte, error) {
	exe, err := memexec.New(otel_contrib)
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

func New(logger *zap.Logger, policyName string, policyDir string, config *config.Config) Runner {
	return Runner{
		logger: logger, policyName: policyName, policyDir: policyDir,
		selfTelemetry: config.SelfTelemetry, sets: config.Set, featureGates: config.FeatureGates, errChan: make(chan string),
	}
}

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

func (r *Runner) Start(ctx context.Context, cancelFunc context.CancelFunc) error {
	r.cancelFunc = cancelFunc
	r.ctx = ctx

	exe, err := memexec.New(otel_contrib)
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
		r.setStatus(Running)
		r.logger.Info("runner proccess started successfully", zap.String("policy", r.policyName), zap.Any("pid", r.cmd.Process.Pid))
	}

	go func() {
		for {
			select {
			case line := <-r.errChan:
				r.state.LastError = string(append([]byte("otelcol-contrib - "), reg.ReplaceAllString(line, "")...))
				r.setStatus(RunnerError)
			case <-r.ctx.Done():
				r.Stop(r.ctx)
				return
			}
		}
	}()

	return nil
}

func (r *Runner) Stop(ctx context.Context) {
	r.logger.Info("routine call to stop runner", zap.Any("routine", ctx.Value("routine")))
	defer r.cancelFunc()
	r.setStatus(Offline)
	r.logger.Info("runner process stopped", zap.String("policy", r.policyName))
}

func (r *Runner) GetStatus() State {
	return r.state
}

func (r *Runner) setStatus(s Status) {
	r.state.Status = s
	r.state.StatusText = MapStatus[s]
}
