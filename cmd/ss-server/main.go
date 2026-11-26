package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/icebase/mu/internal/ssserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ss-server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := ssserver.ConfigFromEnv()
	if err != nil {
		return err
	}
	server, err := ssserver.New(config)
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()
	if err := server.Start(); err != nil {
		return err
	}

	apiListener, err := net.Listen("tcp", config.APIAddr)
	if err != nil {
		return err
	}
	api := ssserver.NewAPIServer(server, config.APIToken)
	apiErr := make(chan error, 1)
	go func() { apiErr <- ssserver.ServeAPI(api, apiListener) }()

	var metrics *http.Server
	if config.MetricsEnabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metrics = &http.Server{Addr: ":" + config.MetricsPort, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := metrics.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("metrics server failed", "error", err)
			}
		}()
	}

	slog.Info("ss-server started", "listen", config.ListenAddr, "method", config.Method, "api", config.APIAddr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var result error
	select {
	case <-ctx.Done():
	case result = <-apiErr:
	}
	api.GracefulStop()
	if metrics != nil {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metrics.Shutdown(shutdown)
	}
	return result
}
