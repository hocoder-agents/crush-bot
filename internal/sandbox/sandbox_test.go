package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

func TestRequired(t *testing.T) {
	if Required(roster.Bot{}) {
		t.Fatal("default bot must not require sandbox")
	}
	if !Required(roster.Bot{Tools: roster.Tools{Bash: true}}) {
		t.Fatal("bash bot requires sandbox")
	}
	if Required(roster.Bot{Tools: roster.Tools{Bash: true}, Sandbox: "off"}) {
		t.Fatal("sandbox:off")
	}
}

func TestBwrapArgsNoHomeBind(t *testing.T) {
	root := t.TempDir()
	bot := roster.Bot{Slug: "coder", Tools: roster.Tools{Bash: true, Edit: true}}
	args := BwrapArgs("/usr/bin/crush", []string{"run", "hi"}, bot, root)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--die-with-parent") {
		t.Fatalf("%s", joined)
	}
	if !strings.Contains(joined, "--bind") {
		t.Fatal("missing rw bind")
	}
	// operator HOME must not appear as a bind source besides sandbox-home under bot
	for i, a := range args {
		if a == "--bind" || a == "--ro-bind" {
			src := args[i+1]
			if src == osHome(t) {
				t.Fatalf("bound operator HOME %s", src)
			}
		}
	}
}

func TestCrushRuntimePathsNpmLayout(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "lib", "node_modules", "@charmland", "crush")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lib.js", "run-crush.js", "package.json"} {
		if err := os.WriteFile(filepath.Join(pkg, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pkg, "bin", "crush"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	bindir := filepath.Join(root, "bin")
	if err := os.MkdirAll(bindir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bindir, "crush")
	if err := os.Symlink(filepath.Join(pkg, "run-crush.js"), link); err != nil {
		t.Fatal(err)
	}
	paths := crushRuntimePaths(link)
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, pkg) {
		t.Fatalf("package dir missing from %v", paths)
	}
}

func TestBwrapArgsBindsCrushPackageDir(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "crush-pkg")
	if err := os.MkdirAll(pkg, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(pkg, "run-crush.js")
	if err := os.WriteFile(script, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "lib.js"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bot := roster.Bot{Slug: "coder", Tools: roster.Tools{Bash: true}}
	args := BwrapArgs(script, []string{"run", "hi"}, bot, root)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, pkg) {
		t.Fatalf("expected ro-bind of package dir in %s", joined)
	}
}

func TestCrushHostPathsIncludesDataDir(t *testing.T) {
	paths := crushHostPaths()
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, filepath.Join("share", "crush")) && !strings.Contains(joined, "crush") {
		t.Fatalf("data dir missing: %v", paths)
	}
	home := osHome(t)
	for _, p := range paths {
		if p == home {
			t.Fatalf("must not bind operator HOME: %v", paths)
		}
	}
}

func osHome(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "/nonexistent-home"
	}
	return h
}

func TestCrushHostDirPairsRebaseIntoSandboxHome(t *testing.T) {
	home := osHome(t)
	root := t.TempDir()
	bot := roster.Bot{Slug: "coder", Tools: roster.Tools{Bash: true}}
	sandboxHome := filepath.Join(roster.Home(root, bot.Slug), "sandbox-home")
	pairs := crushHostDirPairs(sandboxHome)
	joined := ""
	for _, p := range pairs {
		joined += p[0] + " -> " + p[1] + "\n"
	}
	want := filepath.Join(home, ".local/share", "crush")
	if !strings.Contains(joined, want+" -> "+filepath.Join(sandboxHome, ".local/share", "crush")) {
		t.Fatalf("expected %s rebased under sandbox home, got:\n%s", want, joined)
	}
	if !strings.Contains(joined, "/etc/crush -> /etc/crush") {
		t.Fatalf("missing /etc/crush pair:\n%s", joined)
	}
}

func TestRoundGroupDirBoundRW(t *testing.T) {
	root := t.TempDir()
	bot := roster.Bot{Slug: "diana", Tools: roster.Tools{Bash: true, Edit: true}}
	home := roster.Home(root, bot.Slug)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	gid := "review"
	if err := os.WriteFile(filepath.Join(home, "turn.json"), []byte(`{"kind":"group_round","group_id":"`+gid+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := BwrapArgs("/usr/bin/crush", []string{"run", "hi"}, bot, root)
	joined := strings.Join(args, " ")
	dir := filepath.Join(root, "groups", gid)
	if !strings.Contains(joined, "--bind "+dir+" "+dir) {
		t.Fatalf("room dir not RW-bound:\n%s", joined)
	}
	// landlock path
	if got := roundGroupID(home); got != gid {
		t.Fatalf("roundGroupID = %q", got)
	}
}
