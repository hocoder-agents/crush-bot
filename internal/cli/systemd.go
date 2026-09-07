package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hocoder-agents/crush-bot/internal/config"
)

func unitPath() (string, error) {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfg = filepath.Join(h, ".config")
	}
	return filepath.Join(cfg, "systemd", "user", "crushbot.service"), nil
}

func unitBody(exe, home string) string {
	path := filepath.Dir(exe) + ":/usr/local/bin:/usr/bin:/bin"
	return fmt.Sprintf(`[Unit]
Description=crushbot mesh daemon
After=default.target

[Service]
Type=simple
ExecStart=%s daemon run
Restart=on-failure
RestartSec=2
Environment=CRUSHBOT_HOME=%s
Environment=PATH=%s

[Install]
WantedBy=default.target
`, exe, home, path)
}

func daemonInstall(io IO) int {
	p := config.ResolvePaths()
	if err := config.EnsureHome(p); err != nil {
		return fail(io, err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fail(io, err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	path, err := unitPath()
	if err != nil {
		return fail(io, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail(io, err)
	}
	if err := os.WriteFile(path, []byte(unitBody(exe, p.Home)), 0o644); err != nil {
		return fail(io, err)
	}
	fmt.Fprintln(io.Out, okStyle.Render("wrote "+path))
	if _, err := exec.LookPath("systemctl"); err != nil {
		fmt.Fprintln(io.Out, mutedStyle.Render("systemctl not found; enable later with: systemctl --user enable --now crushbot.service"))
		return 0
	}
	cmds := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "crushbot.service"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintln(io.Err, mutedStyle.Render(strings.TrimSpace(string(out))+" ("+err.Error()+")"))
			fmt.Fprintln(io.Out, mutedStyle.Render("unit installed; run: systemctl --user enable --now crushbot.service"))
			return 0
		}
	}
	fmt.Fprintln(io.Out, okStyle.Render("enabled --now crushbot.service"))
	return 0
}

func daemonUninstall(io IO) int {
	path, err := unitPath()
	if err != nil {
		return fail(io, err)
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "disable", "--now", "crushbot.service").Run()
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fail(io, err)
	}
	fmt.Fprintln(io.Out, okStyle.Render("removed user unit"))
	return 0
}
