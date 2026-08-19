// Package config handles the zammad CLI configuration file and instance
// (profile) resolution.
//
// Resolution order for the active instance:
//  1. ZAMMAD_URL + ZAMMAD_TOKEN environment variables (full override, no file needed)
//  2. --instance flag
//  3. ZAMMAD_INSTANCE environment variable
//  4. "default" key in the config file
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	EnvURL      = "ZAMMAD_URL"
	EnvToken    = "ZAMMAD_TOKEN"
	EnvInstance = "ZAMMAD_INSTANCE"
	EnvConfig   = "ZAMMAD_CONFIG" // override config file path
)

type Instance struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type Config struct {
	Default   string              `yaml:"default,omitempty"`
	Instances map[string]Instance `yaml:"instances,omitempty"`
}

// Path returns the config file location: $ZAMMAD_CONFIG or
// <os.UserConfigDir>/zammad/config.yml.
func Path() (string, error) {
	if p := os.Getenv(EnvConfig); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "zammad", "config.yml"), nil
}

// Load reads the config file. A missing file yields an empty config, not an error.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{Instances: map[string]Instance{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if cfg.Instances == nil {
		cfg.Instances = map[string]Instance{}
	}
	return &cfg, nil
}

// Save writes the config file with 0600 permissions, creating the directory
// if needed.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Resolve returns the URL and token for the active instance. flagInstance is
// the value of --instance ("" if unset).
func Resolve(flagInstance string) (name, url, token string, err error) {
	if u, t := os.Getenv(EnvURL), os.Getenv(EnvToken); u != "" && t != "" {
		return "(env)", u, t, nil
	}
	cfg, err := Load()
	if err != nil {
		return "", "", "", err
	}
	name = flagInstance
	if name == "" {
		name = os.Getenv(EnvInstance)
	}
	if name == "" {
		name = cfg.Default
	}
	if name == "" {
		return "", "", "", fmt.Errorf("not logged in: run `zammad auth login` or set %s and %s", EnvURL, EnvToken)
	}
	inst, ok := cfg.Instances[name]
	if !ok {
		return "", "", "", fmt.Errorf("unknown instance %q in config (available: %s)", name, keys(cfg.Instances))
	}
	if inst.URL == "" || inst.Token == "" {
		return "", "", "", fmt.Errorf("instance %q is missing url or token: run `zammad auth login`", name)
	}
	return name, inst.URL, inst.Token, nil
}

func keys(m map[string]Instance) string {
	if len(m) == 0 {
		return "none"
	}
	s := ""
	for k := range m {
		if s != "" {
			s += ", "
		}
		s += k
	}
	return s
}
