package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonInstallWritesUnit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	var out, errb strings.Builder
	env := IO{Out: &out, Err: &errb}
	code := daemonInstall(env)
	if code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	p := filepath.Join(dir, "cfg", "systemd", "user", "crushbot.service")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "daemon run") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, "CRUSHBOT_HOME=") {
		t.Fatalf("%s", body)
	}
	if !strings.Contains(body, "Environment=PATH=") {
		t.Fatalf("%s", body)
	}
}
