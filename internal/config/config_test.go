package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultLimits(t *testing.T) {
	c := Default()
	if c.MaxHops != 8 || c.MaxParallel != 4 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if !c.Experimental.Tasks {
		t.Fatal("tasks default on for new inits")
	}
	if c.MinCrushVersion != MinCrushVersion {
		t.Fatalf("min crush: %s", c.MinCrushVersion)
	}
}

func TestResolvePathsXDG(t *testing.T) {
	t.Setenv("HOME", "/tmp/crushbot-home-test")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/crushbot-xdg-config")
	t.Setenv("XDG_DATA_HOME", "/tmp/crushbot-xdg-data")
	t.Setenv("CRUSHBOT_HOME", "")
	os.Unsetenv("CRUSHBOT_HOME")
	p := ResolvePaths()
	if p.ConfigFile != filepath.Join("/tmp/crushbot-xdg-config", "crushbot", "config.yaml") {
		t.Fatalf("config file: %s", p.ConfigFile)
	}
	if p.Home != filepath.Join("/tmp/crushbot-xdg-data", "crushbot") {
		t.Fatalf("home: %s", p.Home)
	}
}

func TestCRUSHBOT_HOMEOverride(t *testing.T) {
	t.Setenv("CRUSHBOT_HOME", "/tmp/cb-override")
	p := ResolvePaths()
	if p.Home != "/tmp/cb-override" {
		t.Fatalf("home: %s", p.Home)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		ConfigDir:  filepath.Join(dir, "cfg"),
		ConfigFile: filepath.Join(dir, "cfg", "config.yaml"),
		Home:       filepath.Join(dir, "home"),
	}
	cfg := Default()
	cfg.MaxHops = 3
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxHops != 3 {
		t.Fatalf("max hops: %d", got.MaxHops)
	}
	if got.TurnLockTimeout != 120*time.Second {
		t.Fatalf("timeout: %s", got.TurnLockTimeout)
	}
}

func TestEnsureHome(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		ConfigDir:  filepath.Join(dir, "cfg"),
		ConfigFile: filepath.Join(dir, "cfg", "config.yaml"),
		Home:       filepath.Join(dir, "home"),
	}
	if err := EnsureHome(p); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"bots", "logs", "config.yaml", "needs_you.jsonl"} {
		path := filepath.Join(p.Home, rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
	}
	// idempotent
	if err := EnsureHome(p); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingIsDefault(t *testing.T) {
	p := Paths{ConfigFile: filepath.Join(t.TempDir(), "nope.yaml")}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxBots != 32 {
		t.Fatalf("got %+v", cfg)
	}
}
