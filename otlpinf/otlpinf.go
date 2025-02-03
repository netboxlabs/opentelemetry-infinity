package otlpinf

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/runner"
	"gopkg.in/yaml.v3"
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
}

// New creates a new otlpinf routine
func NewOtlp(logger *slog.Logger, c *config.Config) *OltpInf {
	return &OltpInf{logger: logger, conf: c, policies: make(map[string]RunnerInfo)}
}

// Start starts the otlpinf routine
func (o *OltpInf) Start(ctx context.Context, cancelFunc context.CancelFunc) error {
	o.stat.StartTime = time.Now()
	o.ctx = context.WithValue(ctx, routineKey, "otlpInfRoutine")
	o.cancelFunction = cancelFunc

	var err error
	o.policiesDir, err = os.MkdirTemp("", "policies")
	if err != nil {
		return err
	}
	o.capabilities, err = runner.GetCapabilities(o.logger)
	if err != nil {
		return err
	}
	s := struct {
		Buildinfo struct {
			Version string
		}
	}{}
	err = yaml.Unmarshal(o.capabilities, &s)
	if err != nil {
		return err
	}
	o.stat.Version = s.Buildinfo.Version

	o.startServer()

	return nil
}

// Stop stops the otlpinf routine
func (o *OltpInf) Stop(ctx context.Context) {
	o.logger.Info("routine call for stop otlpinf", slog.Any("routine", ctx.Value("routine")))
	defer func() {
		if err := os.RemoveAll(o.policiesDir); err != nil {
			o.logger.Error("error removing policies directory", "error", err)
		}
	}()
	defer o.cancelFunction()
}
