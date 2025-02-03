package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/otlpinf"
	"github.com/spf13/cobra"
)

const routineKey config.ContextKey = "routine"

var (
	debug         bool
	selfTelemetry bool
	serverHost    string
	serverPort    uint64
	set           []string
	featureGates  string
)

func run(_ *cobra.Command, _ []string) {
	config := config.Config{
		Debug:         debug,
		SelfTelemetry: selfTelemetry,
		ServerHost:    serverHost,
		ServerPort:    serverPort,
		Set:           set,
		FeatureGates:  featureGates,
	}
	// logger
	var logger *slog.Logger
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// new otlpinf
	a := otlpinf.NewOtlp(logger, &config)

	// handle signals
	done := make(chan bool, 1)
	rootCtx, cancelFunc := context.WithCancel(context.WithValue(context.Background(), routineKey, "mainRoutine"))

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		for {
			select {
			case <-sigs:
				logger.Warn("stop signal received, stopping otlpinf")
				a.Stop(rootCtx)
				cancelFunc()
			case <-rootCtx.Done():
				logger.Warn("mainRoutine context cancelled")
				done <- true
				return
			}
		}
	}()

	// start otlpinf
	err := a.Start(rootCtx, cancelFunc)
	if err != nil {
		logger.Error("otlpinf startup error", "error", err)
		os.Exit(1)
	}

	<-done
}

func main() {
	rootCmd := &cobra.Command{
		Use: "opentelemetry-infinity",
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run opentelemetry-infinity",
		Long:  `Run opentelemetry-infinity`,
		Run:   run,
	}

	runCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable verbose (debug level) output")
	runCmd.PersistentFlags().BoolVarP(&selfTelemetry, "self_telemetry", "s", false, "Enable self telemetry for collectors. It is disabled by default to avoid port conflict")
	runCmd.PersistentFlags().StringVarP(&serverHost, "server_host", "a", "localhost", "Define REST Host")
	runCmd.PersistentFlags().Uint64VarP(&serverPort, "server_port", "p", 10222, "Define REST Port")
	runCmd.PersistentFlags().StringSliceVarP(&set, "set", "e", nil, "Define opentelemetry set")
	runCmd.PersistentFlags().StringVarP(&featureGates, "feature_gates", "f", "", "Define opentelemetry feature gates")

	rootCmd.AddCommand(runCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
