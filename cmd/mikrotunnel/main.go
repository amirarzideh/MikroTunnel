package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/api"
	"github.com/amirarzideh/MikroTunnel/internal/config"
	"github.com/amirarzideh/MikroTunnel/internal/controller"
	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"github.com/amirarzideh/MikroTunnel/internal/provider/gre"
	"github.com/amirarzideh/MikroTunnel/internal/security"
	"github.com/amirarzideh/MikroTunnel/internal/store"
	"github.com/amirarzideh/MikroTunnel/internal/system"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: mikrotunnel serve --config /etc/mikrotunnel/config.yaml")
		os.Exit(2)
	}
	serve(os.Args[2:])
}
func serve(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	path := flags.String("config", "/etc/mikrotunnel/config.yaml", "configuration path")
	flags.Parse(args)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(*path)
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	db, err := store.Open(cfg.Storage.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	has, err := db.HasAPIKeys(ctx)
	if err != nil {
		logger.Error("check API keys", "error", err)
		os.Exit(1)
	}
	if !has {
		key, err := security.Create(ctx, db)
		if err != nil {
			logger.Error("create bootstrap API key", "error", err)
			os.Exit(1)
		}
		if err := os.WriteFile(cfg.Security.BootstrapKeyFile, []byte(key+"\n"), 0o600); err != nil {
			logger.Error("write bootstrap API key", "error", err)
			os.Exit(1)
		}
		logger.Warn("bootstrap API key written once; copy it and remove the file", "path", cfg.Security.BootstrapKeyFile)
	}
	started := time.Now()
	server := api.New(db, system.NewInspector(started), logger)
	reconciler := controller.New(db, []domain.TunnelProvider{gre.Provider{}}, logger)
	go reconciler.Run(ctx, cfg.Network.ReconcileInterval)
	httpServer := &http.Server{Addr: cfg.Server.ListenAddress, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Info("MikroTunnel started", "address", cfg.Server.ListenAddress)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
