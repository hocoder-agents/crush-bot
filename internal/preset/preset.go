package preset

import (
	"embed"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed souls/manifest.yaml souls/*.md
var files embed.FS

// Entry is one default bot shipped with crushbot.
type Entry struct {
	Slug        string `yaml:"slug"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Coder       bool   `yaml:"coder"`
}

type manifest struct {
	Presets []Entry `yaml:"presets"`
}

func load() (manifest, error) {
	b, err := files.ReadFile("souls/manifest.yaml")
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return manifest{}, fmt.Errorf("parse souls/manifest.yaml: %w", err)
	}
	return m, nil
}

// List returns every shipped default bot.
func List() ([]Entry, error) {
	m, err := load()
	if err != nil {
		return nil, err
	}
	return m.Presets, nil
}

// Get returns the entry and soul body for a slug, or ok=false.
func Get(slug string) (Entry, string, bool) {
	m, err := load()
	if err != nil {
		return Entry{}, "", false
	}
	for _, e := range m.Presets {
		if e.Slug == slug {
			b, err := files.ReadFile(filepath.ToSlash(filepath.Join("souls", slug+".md")))
			if err != nil {
				return Entry{}, "", false
			}
			return e, string(b), true
		}
	}
	return Entry{}, "", false
}
