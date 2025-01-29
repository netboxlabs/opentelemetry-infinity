package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/otlpinf"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Debug         bool
	SelfTelemetry bool
	ServerHost    string
	ServerPort    uint64
	Set           []string
	FeatureGates  string
)

func Run(cmd *cobra.Command, args []string) {
	config := config.Config{
		Debug:         Debug,
		SelfTelemetry: SelfTelemetry,
		ServerHost:    ServerHost,
		ServerPort:    ServerPort,
		Set:           Set,
		FeatureGates:  FeatureGates,
	}
	// logger
	var logger *zap.Logger
	atomicLevel := zap.NewAtomicLevel()
	if Debug {
		atomicLevel.SetLevel(zap.DebugLevel)
	} else {
		atomicLevel.SetLevel(zap.InfoLevel)
	}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		os.Stdout,
		atomicLevel,
	)
	logger = zap.New(core, zap.AddCaller())
	defer func(logger *zap.Logger) {
		_ = logger.Sync()
	}(logger)

	// new otlpinf
	a, err := otlpinf.New(logger, &config)
	if err != nil {
		logger.Error("otlpinf start up error", zap.Error(err))
		os.Exit(1)
	}

	// handle signals
	done := make(chan bool, 1)
	rootCtx, cancelFunc := context.WithCancel(context.WithValue(context.Background(), "routine", "mainRoutine"))

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
	err = a.Start(rootCtx, cancelFunc)
	if err != nil {
		logger.Error("otlpinf startup error", zap.Error(err))
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
		Run:   Run,
	}

	runCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "Enable verbose (debug level) output")
	runCmd.PersistentFlags().BoolVarP(&SelfTelemetry, "self_telemetry", "s", false, "Enable self telemetry for collectors. It is disabled by default to avoid port conflict")
	runCmd.PersistentFlags().StringVarP(&ServerHost, "server_host", "a", "localhost", "Define REST Host")
	runCmd.PersistentFlags().Uint64VarP(&ServerPort, "server_port", "p", 10222, "Define REST Port")
	runCmd.PersistentFlags().StringSliceVarP(&Set, "set", "e", nil, "Define opentelemetry set")
	runCmd.PersistentFlags().StringVarP(&FeatureGates, "feature_gates", "f", "", "Define opentelemetry feature gates")

	rootCmd.AddCommand(runCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
