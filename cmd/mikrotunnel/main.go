package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
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
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "service":
		service(os.Args[2:])
	case "setup":
		setup(os.Args[2:])
	case "uninstall":
		uninstall(os.Args[2:])
	case "version":
		fmt.Println("MikroTunnel development")
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  mikrotunnel serve --config /etc/mikrotunnel/config.yaml\n  mikrotunnel service <status|start|stop|restart|enable|disable>\n  mikrotunnel setup https\n  mikrotunnel uninstall --yes [--purge]\n  mikrotunnel version")
}

func setup(args []string) {
	if len(args) != 1 || args[0] != "https" {
		usage()
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "setup must be run as root")
		os.Exit(1)
	}
	cmd := exec.Command("/usr/local/lib/mikrotunnel/setup-https.sh")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func service(args []string) {
	if len(args) != 1 {
		usage()
		return
	}
	action := args[0]
	allowed := map[string]bool{"status": true, "start": true, "stop": true, "restart": true, "enable": true, "disable": true}
	if !allowed[action] {
		usage()
		return
	}
	cmd := exec.Command("systemctl", action, "mikrotunnel.service")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func uninstall(args []string) {
	confirmed, purge := false, false
	for _, arg := range args {
		switch arg {
		case "--yes":
			confirmed = true
		case "--purge":
			purge = true
		default:
			usage()
			return
		}
	}
	if !confirmed {
		fmt.Fprintln(os.Stderr, "refusing uninstall without --yes; use --purge to also delete configuration and state")
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "uninstall must be run as root")
		os.Exit(1)
	}
	_ = exec.Command("systemctl", "disable", "--now", "mikrotunnel.service").Run()
	_ = os.Remove("/etc/systemd/system/mikrotunnel.service")
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if purge {
		if err := os.RemoveAll("/etc/mikrotunnel"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.RemoveAll("/var/lib/mikrotunnel"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = os.Remove("/etc/caddy/Caddyfile.d/mikrotunnel.caddy")
		_ = os.RemoveAll("/etc/caddy/mikrotunnel-tls")
		_ = os.RemoveAll("/var/lib/caddy/mikrotunnel-acme")
		_ = os.Remove("/etc/systemd/system/mikrotunnel-ip-certificate-renew.service")
		_ = os.Remove("/etc/systemd/system/mikrotunnel-ip-certificate-renew.timer")
		_ = os.RemoveAll("/usr/local/lib/mikrotunnel")
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "reload", "caddy.service").Run()
	}
	_ = os.Remove("/usr/local/bin/mikrotun")
	_ = os.Remove("/usr/local/bin/mikrotunnel")
	fmt.Println("MikroTunnel removed. Configuration and state were", map[bool]string{true: "purged", false: "preserved"}[purge]+".")
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
