package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	MinCrushVersion = "0.91.2"
	AppName         = "crushbot"
)

// Config is the global crushbot config (XDG + optional copy under CRUSHBOT_HOME).
type Config struct {
	CrushPath       string        `yaml:"crush_path"`
	MinCrushVersion string        `yaml:"min_crush_version"`
	MaxParallel     int           `yaml:"max_parallel"`
	MaxHops         int           `yaml:"max_hops"`
	TurnLockTimeout time.Duration `yaml:"turn_lock_timeout"`
	MaxBots         int           `yaml:"max_bots"`
	SoulMaxBytes    int           `yaml:"soul_max_bytes"`
	MessageMaxChars int           `yaml:"message_max_chars"`
	CoalesceInbox   int           `yaml:"coalesce_inbox"`
	ClaimTTLS       int           `yaml:"claim_ttl_s"`
	QueuedExpire    time.Duration `yaml:"queued_expire"`
	Experimental    Experimental  `yaml:"experimental"`
}

type Experimental struct {
	Tasks bool `yaml:"tasks"`
}

func Default() Config {
	return Config{
		CrushPath:       "crush",
		MinCrushVersion: MinCrushVersion,
		MaxParallel:     4,
		MaxHops:         8,
		TurnLockTimeout: 120 * time.Second,
		MaxBots:         32,
		SoulMaxBytes:    32768,
		MessageMaxChars: 16000,
		CoalesceInbox:   8,
		ClaimTTLS:       900,
		QueuedExpire:    24 * time.Hour,
		Experimental: Experimental{
			Tasks: true,
		},
	}
}

// Load reads the XDG config file if it exists; otherwise returns Default.
func Load(p Paths) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.CrushPath == "" {
		c.CrushPath = d.CrushPath
	}
	if c.MinCrushVersion == "" {
		c.MinCrushVersion = d.MinCrushVersion
	}
	if c.MaxParallel <= 0 {
		c.MaxParallel = d.MaxParallel
	}
	if c.MaxHops <= 0 {
		c.MaxHops = d.MaxHops
	}
	if c.TurnLockTimeout <= 0 {
		c.TurnLockTimeout = d.TurnLockTimeout
	}
	if c.MaxBots <= 0 {
		c.MaxBots = d.MaxBots
	}
	if c.SoulMaxBytes <= 0 {
		c.SoulMaxBytes = d.SoulMaxBytes
	}
	if c.MessageMaxChars <= 0 {
		c.MessageMaxChars = d.MessageMaxChars
	}
	if c.CoalesceInbox <= 0 {
		c.CoalesceInbox = d.CoalesceInbox
	}
	if c.ClaimTTLS <= 0 {
		c.ClaimTTLS = d.ClaimTTLS
	}
	if c.QueuedExpire <= 0 {
		c.QueuedExpire = d.QueuedExpire
	}
}

func Save(p Paths, cfg Config) error {
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := p.ConfigFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, p.ConfigFile); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// EnsureHome creates CRUSHBOT_HOME layout (no Crush exec).
func EnsureHome(p Paths) error {
	dirs := []string{
		p.Home,
		filepath.Join(p.Home, "bots"),
		filepath.Join(p.Home, "logs"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	homeCfg := filepath.Join(p.Home, "config.yaml")
	if _, err := os.Stat(homeCfg); os.IsNotExist(err) {
		cfg, err := Load(p)
		if err != nil {
			return err
		}
		b, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		if err := os.WriteFile(homeCfg, b, 0o600); err != nil {
			return fmt.Errorf("write home config: %w", err)
		}
	}
	needs := filepath.Join(p.Home, "needs_you.jsonl")
	if _, err := os.Stat(needs); os.IsNotExist(err) {
		if err := os.WriteFile(needs, nil, 0o600); err != nil {
			return fmt.Errorf("write needs_you: %w", err)
		}
	}
	return nil
}
