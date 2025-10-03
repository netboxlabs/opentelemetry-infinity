package otlpinf

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/runner"
)

const routineKey config.ContextKey = "routine"

// RunnerInfo represents the runner info
type RunnerInfo struct {
	Policy   config.Policy
	Instance *runner.Runner
}

// OltpInf represents the otlpinf routine
type OltpInf struct {
	logger         *slog.Logger
	conf           *config.Config
	stat           config.Status
	policies       map[string]RunnerInfo
	policiesDir    string
	ctx            context.Context
	cancelFunction context.CancelFunc
	router         *gin.Engine
	capabilities   []byte
	httpServer     *http.Server
}

// NewOtlp creates a new otlpinf routine
func NewOtlp(logger *slog.Logger, c *config.Config) *OltpInf {
	return &OltpInf{logger: logger, conf: c, policies: make(map[string]RunnerInfo)}
}

// Start starts the otlpinf routine
func (o *OltpInf) Start(ctx context.Context, cancelFunc context.CancelFunc) <-chan error {
	o.stat.StartTime = time.Now()
	o.ctx = context.WithValue(ctx, routineKey, "otlpInfRoutine")
	o.cancelFunction = cancelFunc

	var err error
	o.policiesDir, err = os.MkdirTemp("", "policies")
	if err != nil {
		return o.startFailure(err)
	}
	o.capabilities, err = runner.GetCapabilities(o.logger)
	if err != nil {
		return o.startFailure(err)
	}
	s := struct {
		Buildinfo struct {
			Version string
		}
	}{}
	err = yaml.Unmarshal(o.capabilities, &s)
	if err != nil {
		return o.startFailure(err)
	}
	o.stat.Version = s.Buildinfo.Version

	return o.startServer()
}

// Stop stops the otlpinf routine
func (o *OltpInf) Stop(_ context.Context) {
	if o.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := o.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			o.logger.Error("error shutting down HTTP server", "error", err)
		}
		o.httpServer = nil
	}
	if o.policiesDir != "" {
		if err := os.RemoveAll(o.policiesDir); err != nil {
			o.logger.Error("error removing policies directory", "error", err)
		}
		o.policiesDir = ""
	}
	if o.cancelFunction != nil {
		o.cancelFunction()
	}
}

func (o *OltpInf) startFailure(err error) <-chan error {
	if o.policiesDir != "" {
		if rmErr := os.RemoveAll(o.policiesDir); rmErr != nil {
			o.logger.Error("error removing policies directory", "error", rmErr)
		}
		o.policiesDir = ""
	}
	errCh := make(chan error, 1)
	errCh <- err
	close(errCh)
	return errCh
}
