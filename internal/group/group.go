package group

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/roster"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Group struct {
	ID             string    `json:"id" yaml:"id"`
	Name           string    `json:"name" yaml:"name"`
	Members        []string  `json:"members" yaml:"members"`
	MaxRounds      int       `json:"max_rounds" yaml:"max_rounds"`
	MaxMsgsPerSend int       `json:"max_msgs_per_send" yaml:"max_msgs_per_send"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
}

type Line struct {
	TS       time.Time `json:"ts"`
	Seq      int       `json:"seq"`
	Round    int       `json:"round"`
	From     string    `json:"from"`
	Kind     string    `json:"kind"`
	Body     string    `json:"body"`
	Mentions []string  `json:"mentions"`
	Pass     bool      `json:"pass"`
}

func Root(home string) string { return filepath.Join(home, "groups") }
func Dir(home, id string) string {
	return filepath.Join(Root(home), id)
}

// Enabled: groups are baked in; no experimental gate anymore.
func Enabled() bool { return true }

func Load(home, id string) (Group, error) {
	b, err := os.ReadFile(filepath.Join(Dir(home, id), "group.yaml"))
	if err != nil {
		return Group{}, err
	}
	var g Group
	if err := json.Unmarshal(b, &g); err != nil {
		return Group{}, err
	}
	return g, nil
}

func Save(home string, g Group) error {
	if err := os.MkdirAll(Dir(home, g.ID), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Dir(home, g.ID), "group.yaml"), raw, 0o600)
}

func List(home string) ([]Group, error) {
	entries, err := os.ReadDir(Root(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Group
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		g, err := Load(home, e.Name())
		if err != nil {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}

func Create(home string, name string, members []string) (Group, error) {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	if !slugRe.MatchString(id) {
		return Group{}, fmt.Errorf("invalid group id %q", id)
	}
	if len(members) < 2 || len(members) > 6 {
		return Group{}, fmt.Errorf("groups need 2–6 members")
	}
	seen := map[string]bool{}
	for _, m := range members {
		if !roster.Exists(home, m) {
			return Group{}, fmt.Errorf("unknown bot %s", m)
		}
		if seen[m] {
			return Group{}, fmt.Errorf("duplicate member %s", m)
		}
		seen[m] = true
	}
	if _, err := Load(home, id); err == nil {
		return Group{}, fmt.Errorf("group %s already exists", id)
	}
	g := Group{
		ID: id, Name: name, Members: members,
		MaxRounds: 3, MaxMsgsPerSend: 10, CreatedAt: time.Now().UTC(),
	}
	if err := Save(home, g); err != nil {
		return Group{}, err
	}
	_ = os.WriteFile(filepath.Join(Dir(home, id), "transcript.jsonl"), nil, 0o600)
	return g, nil
}

func Delete(home, id string) error {
	return os.RemoveAll(Dir(home, id))
}

func TranscriptPath(home, id string) string {
	return filepath.Join(Dir(home, id), "transcript.jsonl")
}

func AppendLine(home, id string, line Line) error {
	path := TranscriptPath(home, id)
	existing, _ := ReadTranscript(home, id)
	line.Seq = len(existing) + 1
	if line.TS.IsZero() {
		line.TS = time.Now().UTC()
	}
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func ReadTranscript(home, id string) ([]Line, error) {
	b, err := os.ReadFile(TranscriptPath(home, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Line
	for _, raw := range strings.Split(string(b), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var l Line
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func Mentions(body string, members []string) []string {
	var hit []string
	for _, m := range members {
		// Tolerate "@ sophie" (space after the @); it clearly names the bot.
		if strings.Contains(body, "@"+m) || strings.Contains(body, "@"+" "+m) {
			hit = append(hit, m)
		}
	}
	return hit
}

func InScope(body string, members []string) []string {
	ms := Mentions(body, members)
	if len(ms) == 0 {
		return members
	}
	return ms
}

func PassFlag(botHome string) string {
	return filepath.Join(botHome, "group_pass")
}

func SaysPath(home, groupID, slug string) string {
	return filepath.Join(Dir(home, groupID), "says-"+slug+".jsonl")
}
