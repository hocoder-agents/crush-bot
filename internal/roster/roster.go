package roster

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hocoder-agents/crush-bot/internal/soul"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Tools struct {
	Bash     bool     `yaml:"bash" json:"bash"`
	Edit     bool     `yaml:"edit" json:"edit"`
	MCPExtra []string `yaml:"mcp_extra" json:"mcp_extra"`
}

type Bot struct {
	Slug                  string            `yaml:"slug" json:"slug"`
	Title                 string            `yaml:"title" json:"title"`
	Description           string            `yaml:"description" json:"description"`
	CreatedAt             time.Time         `yaml:"created_at" json:"created_at"`
	Hidden                bool              `yaml:"hidden" json:"hidden"`
	Model                 string            `yaml:"model" json:"model"`
	Project               string            `yaml:"project" json:"project"`
	CanonicalSessionID    string            `yaml:"canonical_session_id" json:"canonical_session_id"`
	CanonicalSessionTitle string            `yaml:"canonical_session_title" json:"canonical_session_title"`
	GroupSessions         map[string]string `yaml:"group_sessions" json:"group_sessions"`
	Unattended            string            `yaml:"unattended" json:"unattended"`
	Sandbox               string            `yaml:"sandbox" json:"sandbox"`
	Tools                 Tools             `yaml:"tools" json:"tools"`
	CloneFrom             *string           `yaml:"clone_from" json:"clone_from"`
	SoulSHA256            string            `yaml:"soul_sha256" json:"soul_sha256"`
	KeepAlive             bool              `yaml:"keepalive" json:"keepalive"`
}

type SpawnOpts struct {
	Slug        string
	Title       string
	Description string
	Model       string
	Project     string
	CloneFrom   string
	Coder       bool
	// Tools overrides the {Bash, Edit} pair when set; nil means both follow Coder.
	Tools     *Tools
	Sandbox   string
	KeepAlive bool
	MaxBots   int
	SoulMax   int
	// SoulBody seeds soul.md when set; otherwise the generic seed is used.
	SoulBody string
}

func ValidSlug(slug string) bool {
	return slugRe.MatchString(slug)
}

func Home(root, slug string) string {
	return filepath.Join(root, "bots", slug)
}

func yamlPath(root, slug string) string {
	return filepath.Join(Home(root, slug), "bot.yaml")
}

func SoulPath(root, slug string) string {
	return filepath.Join(Home(root, slug), "soul.md")
}

func List(root string, includeHidden bool) ([]Bot, error) {
	entries, err := os.ReadDir(filepath.Join(root, "bots"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Bot
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := Load(root, e.Name())
		if err != nil {
			continue
		}
		if b.Hidden && !includeHidden {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func Load(root, slug string) (Bot, error) {
	b, err := os.ReadFile(yamlPath(root, slug))
	if err != nil {
		return Bot{}, err
	}
	var bot Bot
	if err := yaml.Unmarshal(b, &bot); err != nil {
		return Bot{}, fmt.Errorf("parse bot.yaml: %w", err)
	}
	return bot, nil
}

func Save(root string, bot Bot) error {
	dir := Home(root, bot.Slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := yaml.Marshal(bot)
	if err != nil {
		return err
	}
	tmp := yamlPath(root, bot.Slug) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, yamlPath(root, bot.Slug))
}

func Exists(root, slug string) bool {
	_, err := os.Stat(yamlPath(root, slug))
	return err == nil
}

func Count(root string) (int, error) {
	all, err := List(root, true)
	return len(all), err
}

func SharedProject(root, project, exceptSlug string) ([]string, error) {
	if project == "" {
		return nil, nil
	}
	all, err := List(root, true)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, b := range all {
		if b.Hidden || b.Slug == exceptSlug {
			continue
		}
		if b.Project != "" && samePath(b.Project, project) {
			slugs = append(slugs, b.Slug)
		}
	}
	return slugs, nil
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}

func layoutDirs(home string) []string {
	return []string{
		home,
		filepath.Join(home, "crushrc.d"),
		filepath.Join(home, "hooks"),
		filepath.Join(home, "memory"),
		filepath.Join(home, "inbox", "pending"),
		filepath.Join(home, "inbox", "processing"),
		filepath.Join(home, "inbox", "archive"),
		filepath.Join(home, "inbox", "failed"),
		filepath.Join(home, "tasks"),
		filepath.Join(home, "logs"),
		filepath.Join(home, "skills"),
	}
}

func ensureLayout(home string) error {
	for _, d := range layoutDirs(home) {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	userRC := filepath.Join(home, "crushrc.d", "90-user.crushrc")
	if _, err := os.Stat(userRC); os.IsNotExist(err) {
		if err := os.WriteFile(userRC, []byte("# user crushrc — crushbot never overwrites this file\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func Spawn(root string, opts SpawnOpts) (Bot, []string, error) {
	var warns []string
	if !ValidSlug(opts.Slug) {
		return Bot{}, nil, fmt.Errorf("invalid slug %q (use ^[a-z][a-z0-9-]{0,62}$)", opts.Slug)
	}
	if Exists(root, opts.Slug) {
		return Bot{}, nil, fmt.Errorf("bot %s already exists", opts.Slug)
	}
	n, err := Count(root)
	if err != nil {
		return Bot{}, nil, err
	}
	max := opts.MaxBots
	if max <= 0 {
		max = 32
	}
	if n >= max {
		return Bot{}, nil, fmt.Errorf("roster full (%d bots)", max)
	}

	home := Home(root, opts.Slug)
	if err := ensureLayout(home); err != nil {
		return Bot{}, nil, err
	}

	title := opts.Title
	if title == "" {
		title = strings.ToUpper(opts.Slug[:1]) + opts.Slug[1:]
	}

	var cloneFrom *string
	if opts.CloneFrom != "" {
		src, err := Load(root, opts.CloneFrom)
		if err != nil {
			return Bot{}, nil, fmt.Errorf("clone-from: %w", err)
		}
		cloneFrom = &opts.CloneFrom
		if opts.Description == "" {
			opts.Description = src.Description
		}
		if opts.Model == "" {
			opts.Model = src.Model
		}
		if opts.Project == "" {
			opts.Project = src.Project
		}
		srcSoul := SoulPath(root, opts.CloneFrom)
		dstSoul := SoulPath(root, opts.Slug)
		body, err := os.ReadFile(srcSoul)
		if err != nil {
			return Bot{}, nil, err
		}
		if err := os.WriteFile(dstSoul, body, 0o600); err != nil {
			return Bot{}, nil, err
		}
	}

	soulPath := SoulPath(root, opts.Slug)
	if opts.SoulBody != "" && opts.CloneFrom == "" {
		if err := os.WriteFile(soulPath, []byte(opts.SoulBody), 0o600); err != nil {
			return Bot{}, nil, err
		}
	} else if _, err := soul.WriteSeed(soulPath, opts.Slug); err != nil {
		return Bot{}, nil, err
	}

	body, err := soul.Read(soulPath, opts.SoulMax)
	if err != nil {
		return Bot{}, nil, err
	}
	if hits := soul.Scan(body); len(hits) > 0 {
		warns = append(warns, "soul.md scan: "+strings.Join(hits, ", "))
	}

	if opts.Project != "" {
		if !filepath.IsAbs(opts.Project) {
			return Bot{}, warns, fmt.Errorf("project must be an absolute path")
		}
		if share, err := SharedProject(root, opts.Project, opts.Slug); err != nil {
			return Bot{}, warns, err
		} else if len(share) > 0 {
			warns = append(warns, "shared project with "+strings.Join(share, ", "))
		}
	}

	tools := Tools{Bash: opts.Coder, Edit: opts.Coder}
	if opts.Tools != nil {
		tools = *opts.Tools
	}
	bot := Bot{
		Slug:                  opts.Slug,
		Title:                 title,
		Description:           opts.Description,
		CreatedAt:             time.Now().UTC(),
		Hidden:                false,
		Model:                 opts.Model,
		Project:               opts.Project,
		CanonicalSessionTitle: "bot:" + opts.Slug,
		GroupSessions:         map[string]string{},
		Unattended:            "allowlist",
		Sandbox:               "auto",
		Tools:                 tools,
		CloneFrom:             cloneFrom,
		SoulSHA256:            soul.SHA256(body),
		KeepAlive:             opts.KeepAlive,
	}
	if opts.Sandbox != "" {
		bot.Sandbox = opts.Sandbox
	}
	if err := Save(root, bot); err != nil {
		return Bot{}, warns, err
	}
	return bot, warns, nil
}

// SetProject points a bot's advisory project at an absolute directory
// (or clears it with ""). The caller regenerates protocol files so the
// change reaches the bot's context.
func SetProject(root, slug, project string) (Bot, error) {
	bot, err := Load(root, slug)
	if err != nil {
		return Bot{}, err
	}
	if project != "" && !filepath.IsAbs(project) {
		return Bot{}, fmt.Errorf("project must be an absolute path")
	}
	bot.Project = project
	return bot, Save(root, bot)
}

func SetHidden(root, slug string, hidden bool) (Bot, error) {
	bot, err := Load(root, slug)
	if err != nil {
		return Bot{}, err
	}
	bot.Hidden = hidden
	return bot, Save(root, bot)
}

func Delete(root, slug string) error {
	if !Exists(root, slug) {
		return fmt.Errorf("unknown bot %s", slug)
	}
	return os.RemoveAll(Home(root, slug))
}

func Clone(root, src, dst string, maxBots, soulMax int) (Bot, []string, error) {
	return Spawn(root, SpawnOpts{
		Slug:      dst,
		CloneFrom: src,
		MaxBots:   maxBots,
		SoulMax:   soulMax,
	})
}

func RefreshSoulHash(root, slug string, maxBytes int) (Bot, []string, error) {
	bot, err := Load(root, slug)
	if err != nil {
		return Bot{}, nil, err
	}
	body, err := soul.Read(SoulPath(root, slug), maxBytes)
	if err != nil {
		return Bot{}, nil, err
	}
	var warns []string
	if hits := soul.Scan(body); len(hits) > 0 {
		warns = append(warns, "soul.md scan: "+strings.Join(hits, ", "))
	}
	bot.SoulSHA256 = soul.SHA256(body)
	return bot, warns, Save(root, bot)
}
