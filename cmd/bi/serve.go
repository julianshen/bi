package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/julianshen/bi/internal/config"
	"github.com/julianshen/bi/internal/server"
	"github.com/prometheus/client_golang/prometheus"
)

func runServe(_ []string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(envMap())
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}
	if cfg.LOKPath == "" {
		path, err := config.ResolveLOKPath(config.LOKPathSources{
			Defaults: config.PlatformDefaults(),
		})
		if err != nil {
			logger.Error("resolve lok path", "err", err)
			os.Exit(1)
		}
		cfg.LOKPath = path
	}

	// Each conversion runs in a child `bi convert` process so a LO/cgo
	// crash isolates to one request instead of taking down the server
	// (issue #3). The HTTP path never loads lok in-process.
	exe, err := os.Executable()
	if err != nil {
		logger.Error("locate self", "err", err)
		os.Exit(1)
	}

	reg := prometheus.NewRegistry()
	metrics := server.NewMetrics(reg)

	conv := &server.SubprocessConverter{
		BinPath: exe,
		LOKPath: cfg.LOKPath,
		Timeout: cfg.ConvertTimeout,
		Metrics: metrics,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracer, terr := server.InitTracing(ctx, "bi")
	if terr != nil {
		logger.Warn("tracing disabled", "err", terr)
	} else {
		defer shutdownTracer(context.Background())
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: server.New(server.Deps{
			Conv:           conv,
			Logger:         logger,
			APIToken:       cfg.APIToken,
			MaxUploadBytes: cfg.MaxUploadBytes,
			ReadyzTTL:      cfg.ReadyzCacheTTL,
			Registry:       reg,
			Gatherer:       reg,
			Metrics:        metrics,
		}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("listening", "addr", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("listen", "err", err)
		os.Exit(1)
	}
}

func envMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}
