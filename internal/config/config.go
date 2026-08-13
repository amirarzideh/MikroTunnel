package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		ListenAddress string `yaml:"listen_address"`
	} `yaml:"server"`
	Storage struct {
		DatabasePath string `yaml:"database_path"`
	} `yaml:"storage"`
	Security struct {
		BootstrapKeyFile string `yaml:"bootstrap_key_file"`
	} `yaml:"security"`
	Network struct {
		ReconcileInterval time.Duration `yaml:"-"`
		IntervalRaw       string        `yaml:"reconcile_interval"`
	} `yaml:"network"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Server.ListenAddress == "" || cfg.Storage.DatabasePath == "" || cfg.Security.BootstrapKeyFile == "" {
		return Config{}, errors.New("server.listen_address, storage.database_path and security.bootstrap_key_file are required")
	}
	if cfg.Network.IntervalRaw == "" {
		cfg.Network.IntervalRaw = "20s"
	}
	cfg.Network.ReconcileInterval, err = time.ParseDuration(cfg.Network.IntervalRaw)
	if err != nil || cfg.Network.ReconcileInterval < 5*time.Second {
		return Config{}, errors.New("network.reconcile_interval must be at least 5s")
	}
	for _, target := range []string{cfg.Storage.DatabasePath, cfg.Security.BootstrapKeyFile} {
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}
