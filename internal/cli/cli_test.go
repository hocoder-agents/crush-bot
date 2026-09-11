package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(IO{Out: &out, Err: &errb}, []string{"--help"})
	if code != 0 {
		t.Fatalf("exit %d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "crushbot") {
		t.Fatalf("help: %s", out.String())
	}
	if strings.Contains(strings.ToLower(out.String()), "cobra") {
		t.Fatal("help must not mention cobra")
	}
	if strings.Contains(strings.ToLower(out.String()), "hermes") {
		t.Fatal("help must not mention hermes")
	}
	if !strings.Contains(out.String(), "Charm Crush Powered Bot Mesh") {
		t.Fatalf("help: %s", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(IO{Out: &out, Err: &errb}, []string{"nope"})
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("err: %s", errb.String())
	}
}

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	code := run(IO{Out: &out, Err: &out}, []string{"version"})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), Version) {
		t.Fatalf("version: %s", out.String())
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	var out, errb bytes.Buffer
	code := run(IO{Out: &out, Err: &errb}, []string{"init"})
	if code != 0 {
		t.Fatalf("exit %d err=%s", code, errb.String())
	}
	home := filepath.Join(dir, "home")
	if _, err := os.Stat(filepath.Join(home, "bots")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cfg", "crushbot", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "initialized") {
		t.Fatalf("out: %s", out.String())
	}
}

func TestMeshPlain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	var out, errb bytes.Buffer
	code := run(IO{Out: &out, Err: &errb}, []string{"mesh", "--plain"})
	if code != 0 {
		t.Fatalf("exit %d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "no bots") {
		t.Fatalf("out: %s", out.String())
	}
}

func installFakeCrush(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/usr/bin/env bash
set -euo pipefail
state="${FAKE_CRUSH_STATE:-/tmp/fake-crush-state}"
mkdir -p "$state"
if [[ "${1:-}" == "--version" || "${1:-}" == "-v" ]]; then echo "crush version v0.91.2"; exit 0; fi
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then echo "--yolo"; exit 0; fi
args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version|-v) echo "crush version v0.91.2"; exit 0 ;;
    --help|-h) echo "--yolo"; exit 0 ;;
    --yolo|-y|--debug|-d|--quiet|--continue|-C) shift ;;
    --cwd|-c|--data-dir|-D|--session|-s|--model|--host|-H) shift 2 ;;
    --json) shift ;;
    *) args+=("$1"); shift ;;
  esac
done
set -- "${args[@]+"${args[@]}"}"
cmd="${1:-}"; shift || true
case "$cmd" in
  models) echo "fake/test-model" ;;
  run) echo "11111111-1111-4111-8111-111111111111" > "$state/u"; echo "online" ;;
  server) trap 'exit 0' TERM INT; while true; do sleep 0.2; done ;;
  session)
    sub="${1:-}"; shift || true
    case "$sub" in
      last) uuid=$(cat "$state/u" 2>/dev/null || echo "11111111-1111-4111-8111-111111111111")
            printf '{"meta":{"id":"abc","uuid":"%s","title":"Untitled Session"}}\n' "$uuid" ;;
      rename) echo "$2" > "$state/t" ;;
      show) printf '%s\n' '{"messages":[{"role":"assistant","parts":[{"type":"text","text":"online"}]}]}' ;;
      *) exit 1 ;;
    esac ;;
  *) echo "fake crush tui"; exit 0 ;;
esac
`
	p := filepath.Join(dir, "crush")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CRUSH_STATE", filepath.Join(dir, "st"))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSpawnListShowDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	io := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	if code := run(io, []string{"init"}); code != 0 {
		t.Fatalf("init %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(io, []string{"spawn", "researcher", "--title", "Researcher"}); code != 0 {
		t.Fatalf("spawn %d %s", code, errb.String())
	}
	soul := filepath.Join(dir, "home", "bots", "researcher", "soul.md")
	if _, err := os.Stat(soul); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run(io, []string{"list", "--json"}); code != 0 {
		t.Fatalf("list %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"slug": "researcher"`) {
		t.Fatalf("json: %s", out.String())
	}
	out.Reset()
	if code := run(io, []string{"show", "researcher"}); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	if code := run(io, []string{"delete", "researcher", "--yes"}); code != 0 {
		t.Fatalf("delete %d %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "home", "bots", "researcher")); !os.IsNotExist(err) {
		t.Fatalf("still exists: %v", err)
	}
}

func TestSayAndDoctor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	env := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	if code := run(env, []string{"init"}); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"spawn", "coder"}); code != 0 {
		t.Fatalf("spawn %d %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "home", "bots", "coder", "protocol.md")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"say", "coder", "hello"}); code != 0 {
		t.Fatalf("say %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "online") {
		t.Fatalf("say out %s", out.String())
	}
	out.Reset()
	if code := run(env, []string{"doctor", "coder"}); code != 0 {
		t.Fatalf("doctor %d %s %s", code, out.String(), errb.String())
	}
}

func TestMentionBroadcast(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	env := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	run(env, []string{"init"})
	run(env, []string{"spawn", "alpha"})
	run(env, []string{"spawn", "beta"})
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"mention", "alpha", "beta", "please ping them"}); code != 0 {
		t.Fatalf("mention %d %s", code, errb.String())
	}
	out.Reset()
	if code := run(env, []string{"broadcast", "hello all"}); code != 0 {
		t.Fatalf("broadcast %d %s", code, errb.String())
	}
}

func TestGroupEnableCreate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	env := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	run(env, []string{"init"})
	run(env, []string{"spawn", "alpha"})
	run(env, []string{"spawn", "beta"})
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"group", "create", "review", "alpha", "beta"}); code == 0 {
		t.Fatal("create should fail before enable")
	}
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"group", "enable"}); code != 0 {
		t.Fatalf("enable %d %s", code, errb.String())
	}
	out.Reset()
	if code := run(env, []string{"group", "create", "review", "alpha", "beta"}); code != 0 {
		t.Fatalf("create %d %s", code, errb.String())
	}
	out.Reset()
	if code := run(env, []string{"group", "list"}); code != 0 {
		t.Fatal(errb.String())
	}
	if !strings.Contains(out.String(), "review") {
		t.Fatalf("%s", out.String())
	}
}

func TestKeepaliveStartStop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	env := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	run(env, []string{"init"})
	run(env, []string{"spawn", "hot", "--keepalive"})
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"keepalive", "status", "hot"}); code != 0 && !strings.Contains(out.String(), "hot") {
		t.Fatalf("status %d %s %s", code, out.String(), errb.String())
	}
	run(env, []string{"keepalive", "stop", "hot"})
}

func TestPresetsAndCrew(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CRUSHBOT_HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "cfg"))
	installFakeCrush(t)
	var out, errb bytes.Buffer
	env := IO{Out: &out, Err: &errb, In: strings.NewReader("")}
	if code := run(env, []string{"init"}); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	if code := run(env, []string{"presets"}); code != 0 {
		t.Fatalf("presets %d %s %s", code, out.String(), errb.String())
	}
	for _, want := range []string{"researcher", "coder", "reviewer"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("presets missing %s: %s", want, out.String())
		}
	}
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"crew"}); code != 0 {
		t.Fatalf("crew %d %s %s", code, out.String(), errb.String())
	}
	for _, want := range []string{"researcher", "coder", "reviewer"} {
		if !strings.Contains(out.String(), "spawned "+want) {
			t.Fatalf("crew missing %s: %s %s", want, out.String(), errb.String())
		}
	}
	soulBody, err := os.ReadFile(filepath.Join(dir, "home", "bots", "researcher", "soul.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(soulBody), "Researcher") {
		t.Fatalf("preset soul not seeded: %s", soulBody)
	}
	out.Reset()
	errb.Reset()
	if code := run(env, []string{"crew"}); code != 0 {
		t.Fatalf("crew again %d %s %s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "already have @researcher") {
		t.Fatalf("crew rerun: %s %s", out.String(), errb.String())
	}
}
