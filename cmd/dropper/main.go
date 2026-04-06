package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	dropperroot "github.com/vAudience/dropper"
	dropper "github.com/vAudience/dropper/internal"

	"github.com/itsatony/go-version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, dropper.FatalFormat, err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Initialize version from embedded manifest.
	if err := version.Initialize(
		version.WithEmbedded(dropperroot.VersionsYAML),
	); err != nil {
		return fmt.Errorf("%s: %w", dropper.ErrMsgVersionInit, err)
	}

	// 2. Parse flags.
	configPath := flag.String(dropper.FlagConfigName, "", dropper.FlagConfigUsage)
	flag.Parse()

	// 3. Load config (YAML + env overrides).
	cfg, err := dropper.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	// 4. Initialize structured logger.
	info := version.MustGet()
	logger := dropper.NewLogger(cfg.Dropper.Logging, info.Project.Version)

	logger.Info(dropper.LogMsgStarting,
		dropper.LogFieldPort, cfg.Dropper.ListenPort,
		dropper.LogFieldRootDir, cfg.Dropper.RootDir,
		dropper.LogFieldReadonly, cfg.Dropper.Readonly,
	)

	// 5. Create server.
	srv, err := dropper.NewServer(cfg, logger, dropperroot.StaticFS, dropperroot.TemplateFS)
	if err != nil {
		return fmt.Errorf("%s: %w", dropper.ErrMsgServerStart, err)
	}

	// 6. Start server in goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// 7. Wait for interrupt or server error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info(dropper.LogMsgShutdownSignal, dropper.LogFieldSignal, sig.String())
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("%s: %w", dropper.ErrMsgServerStart, err)
		}
	}

	// 8. Graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), dropper.DefaultShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(dropper.ErrMsgShutdownError, dropper.LogFieldError, err)
	}

	logger.Info(dropper.LogMsgShutdownComplete)
	return nil
}
