package runner

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/amenzhinsky/go-memexec"
	"gopkg.in/yaml.v3"

	"github.com/netboxlabs/opentelemetry-infinity/config"
)

//go:embed otelcol-contrib
var otelContrib []byte

// The embedded collector runs from one in-memory copy made by memexec. The copy
// is reference counted: each running collector and each capabilities call
// holds a reference, and the last release frees it.
var (
	collectorMu   sync.Mutex
	collectorExe  *memexec.Exec
	collectorRefs int
)

// acquireCollector returns the shared collector executable, creating it on
// first use. Every successful call must be paired with releaseCollector.
func acquireCollector() (*memexec.Exec, error) {
	collectorMu.Lock()
	defer collectorMu.Unlock()
	if collectorExe == nil {
		exe, err := memexec.New(otelContrib)
		if err != nil {
			return nil, err
		}
		collectorExe = exe
	}
	collectorRefs++
	return collectorExe, nil
}

var errNoCollectorHolder = errors.New("collector released without a holder")

// releaseCollector drops a reference and frees the executable with the last.
func releaseCollector() error {
	collectorMu.Lock()
	defer collectorMu.Unlock()
	if collectorRefs == 0 {
		return errNoCollectorHolder
	}
	collectorRefs--
	if collectorRefs > 0 || collectorExe == nil {
		return nil
	}
	exe := collectorExe
	collectorExe = nil
	return exe.Close()
}

func logCollectorRelease(logger *slog.Logger) {
	if err := releaseCollector(); err != nil {
		logger.Error("failed to release collector executable", "error", err)
	}
}

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
	logger        *slog.Logger
	policyName    string
	policyDir     string
	policyFile    string
	featureGates  string
	sets          []string
	options       []string
	selfTelemetry bool
	stateMu       sync.Mutex // guards state: the process goroutines write it while the API reads it
	state         State
	cancelFunc    context.CancelFunc
	ctx           context.Context
	cmd           *exec.Cmd
	errChan       chan string
}

// GetCapabilities returns the capabilities of the runner
func GetCapabilities(logger *slog.Logger) ([]byte, error) {
	exe, err := acquireCollector()
	if err != nil {
		return nil, err
	}
	defer logCollectorRelease(logger)
	cmd := exe.Command("components")
	ret, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// NewRunner creates a new runner
func NewRunner(logger *slog.Logger, policyName string, policyDir string, config *config.Config) *Runner {
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
		for _, set := range r.sets {
			r.options = append(r.options, "--set="+set)
		}
	}

	return nil
}

// Start starts the runner
func (r *Runner) Start(ctx context.Context, cancelFunc context.CancelFunc) error {
	r.cancelFunc = cancelFunc
	r.ctx = ctx

	exe, err := acquireCollector()
	if err != nil {
		return err
	}
	// The reference passes to the process once it starts; until then any
	// early return gives it back.
	started := false
	defer func() {
		if !started {
			logCollectorRelease(r.logger)
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
			line := scanner.Text()
			if shouldSuppressCollectorLog(line) {
				continue
			}
			r.setLastLog(line)
			msg, level, attrs := parseCollectorLog(line)
			attrs = append([]slog.Attr{slog.String("policy", r.policyName)}, attrs...)
			r.logger.LogAttrs(r.ctx, level, msg, attrs...)
			if r.cmd.Err != nil {
				r.errChan <- line
			}
		}
	}()
	if err = r.cmd.Start(); err != nil {
		return err
	}
	started = true
	go func() {
		err := r.cmd.Wait()
		// Release before reporting the exit: after Stop nothing reads errChan,
		// so the send below can block forever.
		logCollectorRelease(r.logger)
		if err != nil {
			r.errChan <- r.lastLog()
			close(r.errChan)
		}
	}()

	reg, _ := regexp.Compile("[^a-zA-Z0-9:(), ]+")

	r.stateMu.Lock()
	r.state.startTime = time.Now()
	r.stateMu.Unlock()
	ctxTimeout, cancel := context.WithTimeout(r.ctx, 1*time.Second)
	defer cancel()
	select {
	case line := <-r.errChan:
		return errors.New(string(append([]byte("otelcol-contrib - "), reg.ReplaceAllString(line, "")...)))
	case <-ctxTimeout.Done():
		r.setStatus(running)
		r.logger.Info("runner proccess started successfully", slog.String("policy", r.policyName), slog.Any("pid", r.cmd.Process.Pid))
	}

	go func() {
		for {
			select {
			case line := <-r.errChan:
				r.recordError(string(append([]byte("otelcol-contrib - "), reg.ReplaceAllString(line, "")...)))
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
	r.logger.Info("routine call to stop runner", slog.Any("routine", ctx.Value("routine")))
	defer r.cancelFunc()
	r.setStatus(offline)
	r.logger.Info("runner process stopped", slog.String("policy", r.policyName))
}

// GetStatus returns a copy of the status of the runner
func (r *Runner) GetStatus() State {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.state
}

func (r *Runner) setStatus(s status) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.state.Status = s
	r.state.StatusText = mapStatus[s]
}

// recordError stores the collector's error and the error status together.
func (r *Runner) recordError(msg string) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.state.LastError = msg
	r.state.Status = runnerError
	r.state.StatusText = mapStatus[runnerError]
}

func (r *Runner) setLastLog(line string) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.state.LastLog = line
}

func (r *Runner) lastLog() string {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.state.LastLog
}

func parseCollectorLog(line string) (string, slog.Level, []slog.Attr) {
	msg := line
	level := slog.LevelInfo

	if line == "" {
		return msg, level, nil
	}

	parts := strings.SplitN(line, "\t", 5)
	if len(parts) == 1 {
		return strings.TrimSpace(msg), level, nil
	}

	attrs := make([]slog.Attr, 0, len(parts)-1)

	if len(parts) > 1 {
		if lvl := strings.TrimSpace(parts[1]); lvl != "" {
			level = mapCollectorLevel(lvl)
		}
	}
	if len(parts) > 2 {
		if src := strings.TrimSpace(parts[2]); src != "" {
			attrs = append(attrs, slog.String("collector_source", src))
		}
	}
	if len(parts) > 3 {
		if msgPart := strings.TrimSpace(parts[3]); msgPart != "" {
			msgBytes := []byte(msgPart)
			var structured any
			if err := json.Unmarshal(msgBytes, &structured); err == nil {
				attrs = append(attrs, slog.Any("collector_message", structured))
			}
			msg = msgPart
		}
	}
	if len(parts) > 4 {
		payload := strings.TrimSpace(parts[4])
		if payload != "" {
			var structured any
			if err := json.Unmarshal([]byte(payload), &structured); err == nil {
				attrs = append(attrs, slog.Any("collector_payload", structured))
			} else {
				attrs = append(attrs, slog.String("collector_payload", payload))
			}
		}
	}

	return strings.TrimSpace(msg), level, attrs
}

func shouldSuppressCollectorLog(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// Filter noisy collector warnings emitted when using memexec on some platforms.
	switch {
	case strings.Contains(trimmed, "Failed to get executable path: lstat /memfd"):
		return true
	default:
		return false
	}
}

func mapCollectorLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	case "fatal":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
