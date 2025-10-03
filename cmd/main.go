package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/otlpinf"
)

const routineKey config.ContextKey = "routine"

type runOptions struct {
	debug         bool
	selfTelemetry bool
	serverHost    string
	serverPort    uint64
	set           []string
	featureGates  string
	logTimestamp  bool
}

var runOpts runOptions

func run(_ *cobra.Command, _ []string) error {
	cfg := buildConfig(runOpts)
	logger := newLogger(runOpts)

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx := context.WithValue(signalCtx, routineKey, "mainRoutine")

	app := otlpinf.NewOtlp(logger, &cfg)
	serverErrCh := app.Start(ctx, stop)
	if serverErrCh == nil {
		err := errors.New("start returned nil channel")
		logger.Error("otlpinf startup error", "error", err)
		return err
	}

	if err := checkStartup(serverErrCh); err != nil {
		logger.Error("otlpinf startup error", "error", err)
		app.Stop(ctx)
		return err
	}

	go monitorServerErrors(serverErrCh, logger, stop)

	<-ctx.Done()
	logger.Warn("mainRoutine context cancelled")
	app.Stop(ctx)
	return nil
}

func buildConfig(opts runOptions) config.Config {
	return config.Config{
		Debug:         opts.debug,
		SelfTelemetry: opts.selfTelemetry,
		ServerHost:    opts.serverHost,
		ServerPort:    opts.serverPort,
		Set:           opts.set,
		FeatureGates:  opts.featureGates,
		LogTimestamp:  opts.logTimestamp,
	}
}

func newLogger(opts runOptions) *slog.Logger {
	level := slog.LevelInfo
	if opts.debug {
		level = slog.LevelDebug
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	if !opts.logTimestamp {
		handlerOpts.ReplaceAttr = func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		}
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, handlerOpts))
}

func checkStartup(serverErrCh <-chan error) error {
	select {
	case err, ok := <-serverErrCh:
		if !ok {
			return errors.New("server channel closed during startup")
		}
		return err
	default:
		return nil
	}
}

func monitorServerErrors(errCh <-chan error, logger *slog.Logger, cancel context.CancelFunc) {
	for err := range errCh {
		if err == nil {
			continue
		}
		logger.Error("otlpinf server encountered an error", "error", err)
		cancel()
		return
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use: "opentelemetry-infinity",
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run opentelemetry-infinity",
		Long:  `Run opentelemetry-infinity`,
		RunE:  run,
	}

	runCmd.PersistentFlags().BoolVarP(&runOpts.debug, "debug", "d", false, "Enable verbose (debug level) output")
	runCmd.PersistentFlags().BoolVarP(&runOpts.selfTelemetry, "self_telemetry", "s", false, "Enable self telemetry for collectors. It is disabled by default to avoid port conflict")
	runCmd.PersistentFlags().StringVarP(&runOpts.serverHost, "server_host", "a", "localhost", "Define REST Host")
	runCmd.PersistentFlags().Uint64VarP(&runOpts.serverPort, "server_port", "p", 10222, "Define REST Port")
	runCmd.PersistentFlags().StringSliceVarP(&runOpts.set, "set", "e", nil, "Define opentelemetry set")
	runCmd.PersistentFlags().StringVarP(&runOpts.featureGates, "feature_gates", "f", "", "Define opentelemetry feature gates")
	runCmd.PersistentFlags().BoolVar(&runOpts.logTimestamp, "log_timestamp", true, "Include timestamps in logs")

	rootCmd.AddCommand(runCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
